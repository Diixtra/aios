import logging
import threading
from datetime import datetime, timezone
from typing import Any

import psycopg
from pgvector.psycopg import register_vector
from psycopg.types.json import Jsonb
from psycopg_pool import ConnectionPool

from aios_search.parser import NoteChunk

logger = logging.getLogger(__name__)


TABLE = "aios_vault_embeddings"


def _register_pgvector(conn) -> None:
    register_vector(conn)


class Indexer:
    """pgvector-backed vector store for the knox Obsidian vault.

    DDL is owned by Atlas on the forge side; this class only runs DML and
    fails fast on startup if the table or the ``vector`` extension is missing.
    The public interface matches the legacy Qdrant Indexer so api.py,
    watcher.py, and main.py work unchanged.
    """

    def __init__(
        self,
        database_url: str,
        embedder,
        batch_size: int = 100,
    ):
        self._database_url = database_url
        self._pool: ConnectionPool | None = None
        self._embedder = embedder
        self._batch_size = batch_size
        self.last_indexed_at: datetime | None = None
        self._stats_lock = threading.Lock()

    def ensure_collection(self) -> None:
        """Validate Atlas schema, then open the connection pool.

        We check for the ``vector`` extension and the target table using a
        one-shot psycopg.connect so the pool's pgvector adapter (which
        queries ``pg_type`` for the vector OID) does not deadlock against a
        missing extension.
        """
        with psycopg.connect(self._database_url) as conn, conn.cursor() as cur:
            cur.execute("SELECT 1 FROM pg_extension WHERE extname = 'vector'")
            if cur.fetchone() is None:
                raise RuntimeError(
                    "pgvector extension missing on target database; "
                    "Atlas schema has not been applied"
                )
            cur.execute(
                "SELECT 1 FROM information_schema.tables WHERE table_name = %s",
                (TABLE,),
            )
            if cur.fetchone() is None:
                raise RuntimeError(
                    f"table {TABLE} missing; Atlas schema has not been applied"
                )
        self._pool = ConnectionPool(
            conninfo=self._database_url,
            min_size=1,
            max_size=5,
            configure=_register_pgvector,
            open=True,
        )

    def close(self) -> None:
        if self._pool is not None:
            self._pool.close()
            self._pool = None

    def _require_pool(self) -> ConnectionPool:
        if self._pool is None:
            raise RuntimeError(
                "Indexer pool not opened; call ensure_collection() first"
            )
        return self._pool

    def upsert_chunks(self, chunks: list[NoteChunk]) -> None:
        if not chunks:
            return
        texts = [c.content for c in chunks]
        vectors = self._embedder.embed(texts)
        rows: list[tuple] = []
        for chunk, vector in zip(chunks, vectors):
            metadata_payload = {k: v for k, v in chunk.metadata.items() if k != "title"}
            rows.append(
                (
                    chunk.file_path,
                    chunk.chunk_index,
                    chunk.chunk_total,
                    chunk.title,
                    chunk.content,
                    chunk.content_hash,
                    Jsonb(metadata_payload),
                    vector.tolist(),
                )
            )
        sql = (
            f"INSERT INTO {TABLE} "
            "(source_path, chunk_index, chunk_total, title, content, "
            "content_hash, metadata, embedding) "
            "VALUES (%s, %s, %s, %s, %s, %s, %s, %s) "
            "ON CONFLICT (source_path, chunk_index) DO UPDATE SET "
            "chunk_total = EXCLUDED.chunk_total, "
            "title = EXCLUDED.title, "
            "content = EXCLUDED.content, "
            "content_hash = EXCLUDED.content_hash, "
            "metadata = EXCLUDED.metadata, "
            "embedding = EXCLUDED.embedding, "
            "updated_at = now()"
        )
        with self._require_pool().connection() as conn, conn.cursor() as cur:
            for i in range(0, len(rows), self._batch_size):
                cur.executemany(sql, rows[i : i + self._batch_size])
        with self._stats_lock:
            self.last_indexed_at = datetime.now(timezone.utc)
        logger.info("Upserted %d chunks for %s", len(chunks), chunks[0].file_path)

    def delete_by_file_path(self, file_path: str) -> None:
        with self._require_pool().connection() as conn, conn.cursor() as cur:
            cur.execute(
                f"DELETE FROM {TABLE} WHERE source_path = %s",
                (file_path,),
            )
        logger.info("Deleted vectors for %s", file_path)

    def search(
        self,
        query: str,
        limit: int = 5,
        min_score: float = 0.3,
        filters: dict | None = None,
    ) -> list[dict]:
        query_vector = self._embedder.embed([query])[0].tolist()
        params: list[Any] = [query_vector]
        where_parts = self._filter_where_parts(filters, params)
        where_sql = ("WHERE " + " AND ".join(where_parts)) if where_parts else ""
        params.append(query_vector)
        params.append(max(limit * 4, 20))
        sql = (
            "SELECT source_path, title, chunk_index, content, metadata, "
            "1 - (embedding <=> %s::vector) AS score "
            f"FROM {TABLE} "
            f"{where_sql} "
            "ORDER BY embedding <=> %s::vector "
            "LIMIT %s"
        )
        with self._require_pool().connection() as conn, conn.cursor() as cur:
            cur.execute(sql, params)
            rows = cur.fetchall()
        results = [
            self._row_to_result(r)
            for r in rows
            if r[5] is not None and r[5] >= min_score
        ]
        return results[:limit]

    def find_similar(
        self,
        file_path: str,
        limit: int = 5,
        min_score: float = 0.3,
        filters: dict | None = None,
    ) -> list[dict] | None:
        with self._require_pool().connection() as conn, conn.cursor() as cur:
            cur.execute(
                f"SELECT embedding FROM {TABLE} "
                "WHERE source_path = %s AND chunk_index = 0",
                (file_path,),
            )
            row = cur.fetchone()
            if row is None:
                return None
            query_vector = row[0]
            params: list[Any] = [query_vector]
            where_parts = ["source_path <> %s"]
            params.append(file_path)
            where_parts.extend(self._filter_where_parts(filters, params))
            where_sql = "WHERE " + " AND ".join(where_parts)
            params.append(query_vector)
            params.append(max(limit * 4, 20))
            sql = (
                "SELECT source_path, title, chunk_index, content, metadata, "
                "1 - (embedding <=> %s::vector) AS score "
                f"FROM {TABLE} "
                f"{where_sql} "
                "ORDER BY embedding <=> %s::vector "
                "LIMIT %s"
            )
            cur.execute(sql, params)
            rows = cur.fetchall()
        results = [
            self._row_to_result(r)
            for r in rows
            if r[5] is not None and r[5] >= min_score
        ]
        for r in results:
            r.pop("chunk_index", None)
        return results[:limit]

    def get_indexed_files(self) -> dict[str, str]:
        with self._require_pool().connection() as conn, conn.cursor() as cur:
            cur.execute(
                f"SELECT source_path, content_hash FROM {TABLE} WHERE chunk_index = 0"
            )
            return {row[0]: row[1] for row in cur.fetchall()}

    def get_stats(self) -> dict:
        with self._require_pool().connection() as conn, conn.cursor() as cur:
            cur.execute(f"SELECT count(*) FROM {TABLE}")
            (total,) = cur.fetchone()
        return {
            "total_points": total,
            "status": "green",
            "last_indexed_at": self.last_indexed_at.isoformat()
            if self.last_indexed_at
            else None,
        }

    # Internals ---------------------------------------------------------

    def _filter_where_parts(self, filters: dict | None, params: list[Any]) -> list[str]:
        """Translate a filter dict into SQL predicates.

        Qdrant's ``MatchValue`` matched scalar-against-scalar and
        scalar-against-array uniformly. Replicate via jsonb on ``metadata``:

        - scalar column vs scalar filter:  ``metadata->>key = value``
        - array column containing scalar:  ``metadata->key @> to_jsonb(value)``
        """
        if not filters:
            return []
        parts: list[str] = []
        for key, value in filters.items():
            if value is None:
                continue
            parts.append(
                "((metadata->>%s) = %s OR (metadata->%s) @> to_jsonb(%s::text))"
            )
            params.extend([key, str(value), key, str(value)])
        return parts

    def _row_to_result(self, row: tuple) -> dict:
        source_path, title, chunk_index, content, metadata, score = row
        md = metadata or {}
        return {
            "score": round(score, 4),
            "file_path": source_path,
            "title": title,
            "type": md.get("type"),
            "entity": md.get("entity"),
            "status": md.get("status"),
            "chunk_index": chunk_index,
            "snippet": (content or "")[:200],
        }
