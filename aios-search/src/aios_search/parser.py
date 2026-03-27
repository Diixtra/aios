import hashlib
import re
from dataclasses import dataclass, field
from pathlib import Path

import frontmatter


@dataclass
class NoteChunk:
    file_path: str
    title: str
    metadata: dict
    content: str
    content_hash: str
    chunk_index: int
    chunk_total: int


def should_ignore(
    path: Path,
    vault_root: Path,
    ignored_dirs: list[str],
    ignored_files: list[str],
) -> bool:
    rel = path.relative_to(vault_root)
    parts = rel.parts

    if ".sync-conflict-" in path.name:
        return True

    for part in parts[:-1]:
        if part in ignored_dirs:
            return True

    if path.name in ignored_files:
        return True

    return False


def _frontmatter_summary(metadata: dict) -> str:
    parts = []
    if title := metadata.get("title"):
        parts.append(f"Title: {title}")
    if note_type := metadata.get("type"):
        parts.append(f"Type: {note_type}")
    if entity := metadata.get("entity"):
        if isinstance(entity, list):
            parts.append(f"Entity: {', '.join(entity)}")
        else:
            parts.append(f"Entity: {entity}")
    if status := metadata.get("status"):
        parts.append(f"Status: {status}")
    return ". ".join(parts) + ".\n\n" if parts else ""


def _split_by_headings(body: str) -> list[str]:
    sections = re.split(r"(?=^## )", body, flags=re.MULTILINE)
    return [s.strip() for s in sections if s.strip()]


def _split_by_word_window(
    text: str, window: int = 200, overlap: int = 30
) -> list[str]:
    words = text.split()
    if len(words) <= window:
        return [text]

    chunks = []
    start = 0
    while start < len(words):
        end = start + window
        chunk = " ".join(words[start:end])
        chunks.append(chunk)
        start = end - overlap
        if start + overlap >= len(words):
            break

    return chunks


def parse_note(
    path: Path,
    vault_root: Path,
    chunk_size_threshold: int = 1024,
    chunk_word_window: int = 200,
    chunk_word_overlap: int = 30,
) -> list[NoteChunk]:
    raw = path.read_bytes()
    content_hash = hashlib.md5(raw).hexdigest()
    text = raw.decode("utf-8", errors="replace")
    rel_path = str(path.relative_to(vault_root))

    try:
        post = frontmatter.loads(text)
        metadata = dict(post.metadata)
        body = post.content
    except Exception:
        metadata = {}
        body = text

    title = metadata.get("title", path.stem)
    fm_summary = _frontmatter_summary(metadata)

    if len(text.encode("utf-8")) < chunk_size_threshold:
        content = fm_summary + body
        return [
            NoteChunk(
                file_path=rel_path,
                title=title,
                metadata=metadata,
                content=content,
                content_hash=content_hash,
                chunk_index=0,
                chunk_total=1,
            )
        ]

    sections = _split_by_headings(body)
    if len(sections) > 1:
        chunks = []
        for i, section in enumerate(sections):
            content = fm_summary + section
            chunks.append(
                NoteChunk(
                    file_path=rel_path,
                    title=title,
                    metadata=metadata,
                    content=content,
                    content_hash=content_hash,
                    chunk_index=i,
                    chunk_total=len(sections),
                )
            )
        return chunks

    windows = _split_by_word_window(body, chunk_word_window, chunk_word_overlap)
    chunks = []
    for i, window in enumerate(windows):
        content = fm_summary + window
        chunks.append(
            NoteChunk(
                file_path=rel_path,
                title=title,
                metadata=metadata,
                content=content,
                content_hash=content_hash,
                chunk_index=i,
                chunk_total=len(windows),
            )
        )
    return chunks
