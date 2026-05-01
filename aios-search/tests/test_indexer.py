"""Integration tests for the pgvector Indexer.

We stand up a real pgvector/pgvector:pg18 testcontainer and apply the
production schema.sql once per session. Each test runs in a transaction
that is rolled back so assertions stay isolated.
"""

import os

import numpy as np
import psycopg
import pytest
from testcontainers.postgres import PostgresContainer

from aios_search.indexer import TABLE, Indexer
from aios_search.parser import NoteChunk


PGVECTOR_IMAGE = os.getenv("TEST_PGVECTOR_IMAGE", "pgvector/pgvector:pg18")

SCHEMA_SQL = """
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS aios_vault_embeddings (
  id            bigserial   PRIMARY KEY,
  source_path   text        NOT NULL,
  chunk_index   int         NOT NULL,
  chunk_total   int         NOT NULL,
  title         text        NOT NULL,
  content       text        NOT NULL,
  content_hash  text        NOT NULL,
  metadata      jsonb       NOT NULL DEFAULT '{}'::jsonb,
  embedding     vector(384) NOT NULL,
  updated_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (source_path, chunk_index)
);

CREATE INDEX IF NOT EXISTS aios_vault_embeddings_hnsw_cosine
  ON aios_vault_embeddings USING hnsw (embedding vector_cosine_ops);

CREATE INDEX IF NOT EXISTS aios_vault_embeddings_source_path
  ON aios_vault_embeddings (source_path);

CREATE INDEX IF NOT EXISTS aios_vault_embeddings_metadata_gin
  ON aios_vault_embeddings USING GIN (metadata);
"""


@pytest.fixture(scope="session")
def pg_container():
    with PostgresContainer(PGVECTOR_IMAGE, driver=None) as pg:
        url = pg.get_connection_url()
        # testcontainers prepends "postgresql+psycopg2"; strip that.
        dsn = url.replace("postgresql+psycopg2://", "postgresql://")
        with psycopg.connect(dsn, autocommit=True) as conn, conn.cursor() as cur:
            cur.execute(SCHEMA_SQL)
        yield dsn


@pytest.fixture
def clean_db(pg_container):
    dsn = pg_container
    with psycopg.connect(dsn, autocommit=True) as conn, conn.cursor() as cur:
        cur.execute(f"TRUNCATE {TABLE}")
    return dsn


class _DeterministicEmbedder:
    """Embedder stub: returns a unit-ish 384-dim vector derived from each input."""

    def embed(self, texts):
        results = []
        for t in texts:
            vec = np.zeros(384, dtype=np.float32)
            # Hash each char to spread signal across dimensions.
            for i, ch in enumerate(t[:64]):
                vec[i % 384] = (ord(ch) % 97) / 97.0
            results.append(vec)
        return results


def _chunk(
    file_path: str,
    chunk_index: int,
    content: str,
    title: str = "Title",
    metadata: dict | None = None,
) -> NoteChunk:
    return NoteChunk(
        file_path=file_path,
        title=title,
        metadata=metadata or {},
        content=content,
        content_hash=f"hash-{file_path}-{chunk_index}",
        chunk_index=chunk_index,
        chunk_total=1,
    )


@pytest.fixture
def indexer(clean_db):
    idx = Indexer(
        database_url=clean_db,
        embedder=_DeterministicEmbedder(),
        batch_size=50,
    )
    idx.ensure_collection()
    yield idx
    idx.close()


def test_ensure_collection_raises_when_table_missing(pg_container):
    # Point at a database without the schema applied.
    dsn = pg_container
    with psycopg.connect(dsn, autocommit=True) as conn, conn.cursor() as cur:
        cur.execute("CREATE DATABASE empty_db")
    empty_dsn = dsn.rsplit("/", 1)[0] + "/empty_db"
    idx = Indexer(database_url=empty_dsn, embedder=_DeterministicEmbedder())
    try:
        with pytest.raises(RuntimeError, match="Atlas schema has not been applied"):
            idx.ensure_collection()
    finally:
        idx.close()


def test_upsert_and_search_roundtrip(indexer):
    chunks = [
        _chunk(
            "20-Meetings/idox.md",
            0,
            "IDOX migration discussion",
            title="IDOX Meeting",
            metadata={"type": "meeting", "entity": ["diixtra"], "status": "done"},
        ),
        _chunk(
            "10-Projects/website.md",
            0,
            "New marketing website project",
            title="Website",
            metadata={"type": "project", "entity": ["kazie"], "status": "active"},
        ),
    ]
    indexer.upsert_chunks(chunks)

    results = indexer.search("IDOX migration", limit=5, min_score=0.0)
    assert results, "search must return at least one hit"
    assert results[0]["file_path"] == "20-Meetings/idox.md"
    assert results[0]["title"] == "IDOX Meeting"
    assert results[0]["type"] == "meeting"
    assert results[0]["entity"] == ["diixtra"]
    assert results[0]["status"] == "done"
    assert results[0]["chunk_index"] == 0
    assert results[0]["snippet"].startswith("IDOX migration")


def test_upsert_is_idempotent(indexer):
    chunk = _chunk("notes/a.md", 0, "Some content", title="A")
    indexer.upsert_chunks([chunk])
    indexer.upsert_chunks([chunk])
    stats = indexer.get_stats()
    assert stats["total_points"] == 1


def test_upsert_updates_existing_row(indexer):
    indexer.upsert_chunks([_chunk("notes/a.md", 0, "Original", title="A")])
    indexer.upsert_chunks([_chunk("notes/a.md", 0, "Revised", title="A-rev")])
    with psycopg.connect(indexer._database_url) as conn, conn.cursor() as cur:
        cur.execute(
            f"SELECT title, content FROM {TABLE} WHERE source_path = 'notes/a.md'"
        )
        title, content = cur.fetchone()
    assert title == "A-rev"
    assert content == "Revised"


def test_delete_by_file_path(indexer):
    indexer.upsert_chunks(
        [
            _chunk("a.md", 0, "Alpha"),
            _chunk("b.md", 0, "Bravo"),
        ]
    )
    indexer.delete_by_file_path("a.md")
    assert indexer.get_stats()["total_points"] == 1
    files = indexer.get_indexed_files()
    assert "b.md" in files
    assert "a.md" not in files


def test_get_indexed_files_returns_hashes(indexer):
    indexer.upsert_chunks(
        [
            _chunk("a.md", 0, "Alpha"),
            _chunk("b.md", 0, "Bravo"),
        ]
    )
    files = indexer.get_indexed_files()
    assert files == {"a.md": "hash-a.md-0", "b.md": "hash-b.md-0"}


def test_search_filters_scalar_metadata(indexer):
    indexer.upsert_chunks(
        [
            _chunk("a.md", 0, "Note about IDOX", metadata={"type": "meeting"}),
            _chunk("b.md", 0, "Note about IDOX", metadata={"type": "project"}),
        ]
    )
    results = indexer.search(
        "IDOX", limit=5, min_score=0.0, filters={"type": "meeting"}
    )
    assert len(results) == 1
    assert results[0]["file_path"] == "a.md"


def test_search_filters_array_metadata(indexer):
    indexer.upsert_chunks(
        [
            _chunk("a.md", 0, "Project X", metadata={"entity": ["diixtra", "kazie"]}),
            _chunk("b.md", 0, "Project X", metadata={"entity": ["other"]}),
        ]
    )
    results = indexer.search(
        "Project X", limit=5, min_score=0.0, filters={"entity": "diixtra"}
    )
    assert len(results) == 1
    assert results[0]["file_path"] == "a.md"


def test_find_similar_excludes_source(indexer):
    indexer.upsert_chunks(
        [
            _chunk("a.md", 0, "IDOX migration timeline"),
            _chunk("b.md", 0, "IDOX migration milestones"),
        ]
    )
    results = indexer.find_similar("a.md", limit=5, min_score=0.0)
    assert results is not None
    file_paths = [r["file_path"] for r in results]
    assert "a.md" not in file_paths
    assert "b.md" in file_paths
    # Legacy find_similar response MUST NOT include chunk_index
    assert "chunk_index" not in results[0]


def test_find_similar_returns_none_when_not_indexed(indexer):
    result = indexer.find_similar("nonexistent.md")
    assert result is None


def test_get_stats_counts_rows(indexer):
    assert indexer.get_stats()["total_points"] == 0
    indexer.upsert_chunks([_chunk("a.md", 0, "one")])
    indexer.upsert_chunks([_chunk("b.md", 0, "two")])
    stats = indexer.get_stats()
    assert stats["total_points"] == 2
    assert stats["last_indexed_at"] is not None
    assert stats["status"] == "green"


def test_upsert_empty_chunks_is_noop(indexer):
    indexer.upsert_chunks([])
    assert indexer.get_stats()["total_points"] == 0


def test_search_respects_min_score(indexer):
    # All the deterministic embedder's vectors map similar chars to similar
    # coordinates, so a very high min_score should drop unrelated results.
    indexer.upsert_chunks(
        [
            _chunk("a.md", 0, "alpha"),
            _chunk("b.md", 0, "zzzzz"),
        ]
    )
    results = indexer.search("alpha", limit=5, min_score=0.99)
    # Only the self-match (if any) should clear a 0.99 threshold; unrelated
    # vectors must not.
    for r in results:
        assert r["score"] >= 0.99


def test_search_with_none_filter_value_is_ignored(indexer):
    indexer.upsert_chunks(
        [
            _chunk("a.md", 0, "content", metadata={"type": "meeting"}),
        ]
    )
    results = indexer.search("content", limit=5, min_score=0.0, filters={"type": None})
    assert len(results) == 1
