import hashlib
from pathlib import Path
from unittest.mock import MagicMock, call

import pytest

from aios_search.watcher import Watcher
from aios_search.config import Settings


@pytest.fixture
def mock_settings(tmp_vault):
    settings = MagicMock(spec=Settings)
    settings.vault_path = str(tmp_vault)
    settings.ignored_dirs = [".obsidian", "80-Dashboards", "90-Templates", ".stfolder"]
    settings.ignored_files = [".stignore", ".DS_Store"]
    settings.chunk_size_threshold = 1024
    settings.chunk_word_window = 200
    settings.chunk_word_overlap = 30
    settings.embedding_batch_size = 32
    settings.debounce_ms = 500
    return settings


@pytest.fixture
def mock_indexer():
    indexer = MagicMock()
    indexer.get_indexed_files.return_value = {}
    return indexer


@pytest.fixture
def watcher(mock_settings, mock_indexer):
    return Watcher(settings=mock_settings, indexer=mock_indexer)


def test_process_file_calls_indexer(watcher, mock_indexer, tmp_vault):
    path = tmp_vault / "12-CRM" / "Contacts" / "Shah Ali.md"
    watcher._process_file(path)
    mock_indexer.delete_by_file_path.assert_called_once_with("12-CRM/Contacts/Shah Ali.md")
    mock_indexer.upsert_chunks.assert_called_once()


def test_process_file_adds_to_retry_on_failure(watcher, mock_indexer, tmp_vault):
    mock_indexer.upsert_chunks.side_effect = Exception("Qdrant down")
    path = tmp_vault / "12-CRM" / "Contacts" / "Shah Ali.md"
    watcher._process_file(path)
    assert "12-CRM/Contacts/Shah Ali.md" in watcher._retry_set


def test_process_delete(watcher, mock_indexer, tmp_vault):
    path = tmp_vault / "12-CRM" / "Contacts" / "Shah Ali.md"
    watcher._process_delete(path)
    mock_indexer.delete_by_file_path.assert_called_once_with("12-CRM/Contacts/Shah Ali.md")


def test_reconcile_indexes_missing_files(watcher, mock_indexer, tmp_vault):
    mock_indexer.get_indexed_files.return_value = {}
    watcher.reconcile()
    assert mock_indexer.upsert_chunks.call_count > 0
    assert mock_indexer.delete_by_file_path.call_count > 0


def test_reconcile_removes_orphans(watcher, mock_indexer, tmp_vault):
    mock_indexer.get_indexed_files.return_value = {
        "12-CRM/Contacts/Shah Ali.md": hashlib.md5(
            (tmp_vault / "12-CRM" / "Contacts" / "Shah Ali.md").read_bytes()
        ).hexdigest(),
        "deleted-note.md": "stale-hash",
    }
    watcher.reconcile()
    delete_calls = [
        c for c in mock_indexer.delete_by_file_path.call_args_list
        if c == call("deleted-note.md")
    ]
    assert len(delete_calls) == 1


def test_reconcile_skips_unchanged_files(watcher, mock_indexer, tmp_vault):
    indexed = {}
    for path in tmp_vault.rglob("*.md"):
        if not watcher._should_ignore(path):
            rel = str(path.relative_to(tmp_vault))
            indexed[rel] = hashlib.md5(path.read_bytes()).hexdigest()
    mock_indexer.get_indexed_files.return_value = indexed
    watcher.reconcile()
    mock_indexer.upsert_chunks.assert_not_called()


def test_pause_resume(watcher):
    assert not watcher._paused
    watcher.pause()
    assert watcher._paused
    watcher.resume()
    assert not watcher._paused


def test_full_reindex_pauses_and_resumes(watcher, mock_indexer, tmp_vault):
    watcher.full_reindex()
    assert not watcher._paused
    assert mock_indexer.upsert_chunks.call_count > 0
