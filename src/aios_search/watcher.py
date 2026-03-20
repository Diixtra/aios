import hashlib
import logging
import threading
import time
from pathlib import Path

from watchfiles import Change, watch

from aios_search.config import Settings
from aios_search.indexer import Indexer
from aios_search.parser import parse_note, should_ignore

logger = logging.getLogger(__name__)


class Watcher:
    def __init__(self, settings: Settings, indexer: Indexer):
        self._settings = settings
        self._indexer = indexer
        self._vault = Path(settings.vault_path)
        self._retry_set: set[str] = set()
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

    def _should_ignore(self, path: Path) -> bool:
        return should_ignore(
            path,
            self._vault,
            self._settings.ignored_dirs,
            self._settings.ignored_files,
        )

    def _process_file(self, path: Path):
        try:
            chunks = parse_note(
                path,
                self._vault,
                chunk_size_threshold=self._settings.chunk_size_threshold,
                chunk_word_window=self._settings.chunk_word_window,
                chunk_word_overlap=self._settings.chunk_word_overlap,
            )
            rel_path = str(path.relative_to(self._vault))
            self._indexer.delete_by_file_path(rel_path)
            self._indexer.upsert_chunks(
                chunks, batch_size=self._settings.embedding_batch_size
            )
            self._retry_set.discard(rel_path)
        except Exception:
            rel_path = str(path.relative_to(self._vault))
            self._retry_set.add(rel_path)
            logger.exception("Failed to index %s, added to retry set", rel_path)

    def _process_delete(self, path: Path):
        try:
            rel_path = str(path.relative_to(self._vault))
            self._indexer.delete_by_file_path(rel_path)
            self._retry_set.discard(rel_path)
        except Exception:
            logger.exception("Failed to delete vectors for %s", path)

    def _process_retries(self):
        if not self._retry_set:
            return
        for rel_path in list(self._retry_set):
            path = self._vault / rel_path
            if path.is_file():
                logger.info("Retrying %s", rel_path)
                self._process_file(path)

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
            "Reconciliation complete: %d indexed, %d orphans removed",
            len(to_index),
            len(set(indexed) - set(vault_files)),
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
