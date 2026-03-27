import hashlib
from pathlib import Path

from aios_search.parser import NoteChunk, parse_note, should_ignore


def test_parse_short_note_single_chunk(tmp_vault):
    path = tmp_vault / "12-CRM" / "Contacts" / "Shah Ali.md"
    chunks = parse_note(path, tmp_vault)

    assert len(chunks) == 1
    chunk = chunks[0]
    assert chunk.file_path == "12-CRM/Contacts/Shah Ali.md"
    assert chunk.title == "Shah Ali"
    assert chunk.metadata["type"] == "contact"
    assert chunk.metadata["entity"] == ["properties"]
    assert chunk.chunk_index == 0
    assert chunk.chunk_total == 1
    assert "Property sourcing agent" in chunk.content
    assert chunk.content_hash == hashlib.md5(path.read_bytes()).hexdigest()


def test_parse_long_note_splits_on_headings(tmp_vault):
    path = tmp_vault / "20-Meetings" / "2025-10-15 - IT Systems Progress Update.md"
    chunks = parse_note(path, tmp_vault)

    assert len(chunks) == 3
    assert all(c.file_path == "20-Meetings/2025-10-15 - IT Systems Progress Update.md" for c in chunks)
    assert chunks[0].chunk_index == 0
    assert chunks[1].chunk_index == 1
    assert chunks[2].chunk_index == 2
    assert all(c.chunk_total == 3 for c in chunks)
    assert "IT Systems Progress Update" in chunks[0].content
    assert "meeting" in chunks[0].content.lower() or "diixtra" in chunks[0].content.lower()


def test_parse_long_note_no_headings_uses_window(tmp_vault):
    path = tmp_vault / "50-Knowledge" / "Library" / "Business Framework.md"
    chunks = parse_note(path, tmp_vault, chunk_word_window=200, chunk_word_overlap=30)

    assert len(chunks) >= 2
    assert all(c.file_path == "50-Knowledge/Library/Business Framework.md" for c in chunks)
    assert "Business Framework" in chunks[0].content


def test_parse_no_frontmatter(tmp_vault):
    path = tmp_vault / "plain.md"
    chunks = parse_note(path, tmp_vault)

    assert len(chunks) == 1
    assert chunks[0].metadata == {}
    assert chunks[0].title == "plain"
    assert "Just some text" in chunks[0].content


def test_parse_malformed_frontmatter(tmp_vault):
    path = tmp_vault / "malformed.md"
    chunks = parse_note(path, tmp_vault)

    assert len(chunks) == 1
    assert chunks[0].metadata == {}
    assert "Body text" in chunks[0].content


def test_should_ignore(tmp_vault):
    ignored_dirs = [".obsidian", "80-Dashboards", "90-Templates", ".stfolder"]
    ignored_files = [".stignore", ".DS_Store"]

    assert should_ignore(tmp_vault / "80-Dashboards" / "Home.md", tmp_vault, ignored_dirs, ignored_files)
    assert should_ignore(tmp_vault / ".obsidian" / "app.json", tmp_vault, ignored_dirs, ignored_files)
    assert should_ignore(tmp_vault / ".DS_Store", tmp_vault, ignored_dirs, ignored_files)
    assert not should_ignore(tmp_vault / "12-CRM" / "Contacts" / "Shah Ali.md", tmp_vault, ignored_dirs, ignored_files)


def test_should_ignore_sync_conflict(tmp_vault):
    ignored_dirs = []
    ignored_files = []
    conflict_path = tmp_vault / "note.sync-conflict-20260319.md"
    assert should_ignore(conflict_path, tmp_vault, ignored_dirs, ignored_files)
