import hashlib
import logging
import threading
import time
from dataclasses import dataclass
from pathlib import Path

from watchfiles import Change, watch

from aios_search.config import Settings
from aios_search.indexer import Indexer
from aios_search.parser import parse_note, should_ignore

logger = logging.getLogger(__name__)

_RETRY_BASE_SECONDS = 30.0
_RETRY_MAX_SECONDS = 3600.0


@dataclass
class _RetryInfo:
    attempts: int = 0
    next_retry_at: float = 0.0  # time.monotonic() reference
    last_error: str = ""


class Watcher:
    def __init__(self, settings: Settings, indexer: Indexer):
        self._settings = settings
        self._indexer = indexer
        self._vault = Path(settings.vault_path)
        self._retry_state: dict[str, _RetryInfo] = {}
        self._paused = False
        self._stop_event = threading.Event()
        self._thread: threading.Thread | None = None

    def pause(self):
        self._paused = True
        logger.info("Watcher paused")

    def resume(self):
        self._paused = False
        logger.info("Watcher resumed")

    def stop(self):
        self._stop_event.set()
        if self._thread:
            self._thread.join(timeout=5)

    def start(self):
        self._thread = threading.Thread(target=self._run, daemon=True)
        self._thread.start()

    def has_pending_retry(self, rel_path: str) -> bool:
        return rel_path in self._retry_state

    def _should_ignore(self, path: Path) -> bool:
        return should_ignore(
            path,
            self._vault,
            self._settings.ignored_dirs,
            self._settings.ignored_files,
        )

    def _record_failure(self, rel_path: str, exc: Exception) -> None:
        info = self._retry_state.get(rel_path)
        first_failure = info is None
        if info is None:
            info = _RetryInfo()
            self._retry_state[rel_path] = info
        info.attempts += 1
        backoff = min(
            _RETRY_BASE_SECONDS * (2 ** (info.attempts - 1)),
            _RETRY_MAX_SECONDS,
        )
        info.next_retry_at = time.monotonic() + backoff
        info.last_error = f"{type(exc).__name__}: {exc}"
        if first_failure:
            logger.exception(
                "Failed to index %s (first failure, next retry in %ds)",
                rel_path,
                int(backoff),
            )
        else:
            logger.warning(
                "Failed to index %s (attempt %d, next retry in %ds): %s",
                rel_path,
                info.attempts,
                int(backoff),
                info.last_error,
            )

    def _process_file(self, path: Path):
        rel_path = str(path.relative_to(self._vault))
        try:
            chunks = parse_note(
                path,
                self._vault,
                chunk_size_threshold=self._settings.chunk_size_threshold,
                chunk_word_window=self._settings.chunk_word_window,
                chunk_word_overlap=self._settings.chunk_word_overlap,
            )
            self._indexer.delete_by_file_path(rel_path)
            self._indexer.upsert_chunks(chunks)
            self._retry_state.pop(rel_path, None)
        except Exception as exc:
            self._record_failure(rel_path, exc)

    def _process_delete(self, path: Path):
        try:
            rel_path = str(path.relative_to(self._vault))
            self._indexer.delete_by_file_path(rel_path)
            self._retry_state.pop(rel_path, None)
        except Exception:
            logger.exception("Failed to delete vectors for %s", path)

    def _process_retries(self):
        if not self._retry_state:
            return
        now = time.monotonic()
        due = [
            rel_path
            for rel_path, info in list(self._retry_state.items())
            if info.next_retry_at <= now
        ]
        for rel_path in due:
            path = self._vault / rel_path
            if path.is_file():
                logger.info("Retrying %s", rel_path)
                self._process_file(path)
            else:
                self._retry_state.pop(rel_path, None)

    def reconcile(self):
        logger.info("Starting reconciliation...")
        vault_files: dict[str, str] = {}
        for path in self._vault.rglob("*.md"):
            if self._should_ignore(path):
                continue
            rel_path = str(path.relative_to(self._vault))
            content_hash = hashlib.md5(path.read_bytes()).hexdigest()
            vault_files[rel_path] = content_hash

        indexed = self._indexer.get_indexed_files()

        to_index = []
        for rel_path, content_hash in vault_files.items():
            if rel_path not in indexed or indexed[rel_path] != content_hash:
                to_index.append(self._vault / rel_path)

        for rel_path in indexed:
            if rel_path not in vault_files:
                logger.info("Removing orphaned vectors: %s", rel_path)
                self._indexer.delete_by_file_path(rel_path)

        for path in to_index:
            self._process_file(path)

        logger.info(
            "Reconciliation complete: %d indexed, %d orphans removed, %d pending retry",
            len(to_index),
            len(set(indexed) - set(vault_files)),
            len(self._retry_state),
        )

    def full_reindex(self):
        self.pause()
        try:
            for path in self._vault.rglob("*.md"):
                if self._should_ignore(path):
                    continue
                self._process_file(path)
        finally:
            self.resume()

    def _run(self):
        logger.info("Watcher started for %s", self._vault)
        last_retry = time.monotonic()

        for changes in watch(
            self._vault,
            stop_event=self._stop_event,
            debounce=self._settings.debounce_ms,
        ):
            if self._paused:
                continue

            for change_type, path_str in changes:
                path = Path(path_str)

                if not path.suffix == ".md":
                    continue
                if self._should_ignore(path):
                    continue

                if change_type in (Change.added, Change.modified):
                    logger.info("File changed: %s", path)
                    self._process_file(path)
                elif change_type == Change.deleted:
                    logger.info("File deleted: %s", path)
                    self._process_delete(path)

            now = time.monotonic()
            if now - last_retry > 60:
                self._process_retries()
                last_retry = now
