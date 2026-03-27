import logging
import uuid
from datetime import datetime, timezone

from qdrant_client import QdrantClient
from qdrant_client.models import (
    Distance,
    FieldCondition,
    Filter,
    MatchValue,
    PayloadSchemaType,
    PointStruct,
    VectorParams,
)

from aios_search.parser import NoteChunk

logger = logging.getLogger(__name__)


def make_point_id(file_path: str, chunk_index: int) -> str:
    return str(uuid.uuid5(uuid.NAMESPACE_URL, f"{file_path}#{chunk_index}"))


class Indexer:
    def __init__(
        self,
        qdrant_url: str,
        qdrant_api_key: str,
        collection_name: str,
        vector_size: int,
        embedder,
        qdrant_batch_size: int = 100,
    ):
        self._client = QdrantClient(url=qdrant_url, api_key=qdrant_api_key)
        self._collection = collection_name
        self._vector_size = vector_size
        self._embedder = embedder
        self._batch_size = qdrant_batch_size
        self.last_indexed_at: datetime | None = None

    def ensure_collection(self):
        if not self._client.collection_exists(self._collection):
            self._client.create_collection(
                collection_name=self._collection,
                vectors_config=VectorParams(
                    size=self._vector_size, distance=Distance.COSINE
                ),
            )
            logger.info("Created collection %s", self._collection)
        # Ensure payload indexes exist for filtered queries
        for field, schema in [
            ("chunk_index", PayloadSchemaType.INTEGER),
            ("file_path", PayloadSchemaType.KEYWORD),
        ]:
            self._client.create_payload_index(
                collection_name=self._collection,
                field_name=field,
                field_schema=schema,
            )

    def upsert_chunks(self, chunks: list[NoteChunk]):
        if not chunks:
            return
        texts = [c.content for c in chunks]
        vectors = self._embedder.embed(texts)
        points = []
        now = datetime.now(timezone.utc).isoformat()
        for chunk, vector in zip(chunks, vectors):
            payload = {
                "file_path": chunk.file_path,
                "title": chunk.title,
                "chunk_index": chunk.chunk_index,
                "chunk_total": chunk.chunk_total,
                "content": chunk.content,
                "content_hash": chunk.content_hash,
                "updated": now,
                **{k: v for k, v in chunk.metadata.items() if k != "title"},
            }
            points.append(
                PointStruct(
                    id=make_point_id(chunk.file_path, chunk.chunk_index),
                    vector=vector.tolist(),
                    payload=payload,
                )
            )
        for i in range(0, len(points), self._batch_size):
            batch = points[i : i + self._batch_size]
            self._client.upsert(collection_name=self._collection, points=batch)
        self.last_indexed_at = datetime.now(timezone.utc)
        logger.info("Upserted %d chunks for %s", len(chunks), chunks[0].file_path)

    def delete_by_file_path(self, file_path: str):
        self._client.delete(
            collection_name=self._collection,
            points_selector=Filter(
                must=[FieldCondition(key="file_path", match=MatchValue(value=file_path))]
            ),
        )
        logger.info("Deleted vectors for %s", file_path)

    def search(
        self,
        query: str,
        limit: int = 5,
        min_score: float = 0.3,
        filters: dict | None = None,
    ) -> list[dict]:
        query_vector = self._embedder.embed([query])[0]
        must_conditions = []
        if filters:
            for key, value in filters.items():
                if value is not None:
                    must_conditions.append(
                        FieldCondition(key=key, match=MatchValue(value=value))
                    )
        search_filter = Filter(must=must_conditions) if must_conditions else None
        results = self._client.query_points(
            collection_name=self._collection,
            query=query_vector.tolist(),
            limit=limit,
            score_threshold=min_score,
            query_filter=search_filter,
            with_payload=True,
        )
        return [
            {
                "score": round(point.score, 4),
                "file_path": point.payload.get("file_path"),
                "title": point.payload.get("title"),
                "type": point.payload.get("type"),
                "entity": point.payload.get("entity"),
                "status": point.payload.get("status"),
                "chunk_index": point.payload.get("chunk_index"),
                "snippet": point.payload.get("content", "")[:200],
            }
            for point in results.points
        ]

    def find_similar(self, file_path: str, limit: int = 5, min_score: float = 0.3, filters: dict | None = None) -> list[dict] | None:
        results = self._client.scroll(
            collection_name=self._collection,
            scroll_filter=Filter(
                must=[
                    FieldCondition(key="file_path", match=MatchValue(value=file_path)),
                    FieldCondition(key="chunk_index", match=MatchValue(value=0)),
                ]
            ),
            limit=1,
            with_vectors=True,
        )
        points, _ = results
        if not points:
            return None
        query_vector = points[0].vector

        must_not = [FieldCondition(key="file_path", match=MatchValue(value=file_path))]
        must = []
        if filters:
            for key, value in filters.items():
                if value is not None:
                    must.append(FieldCondition(key=key, match=MatchValue(value=value)))

        search_results = self._client.query_points(
            collection_name=self._collection,
            query=query_vector if isinstance(query_vector, list) else query_vector.tolist(),
            limit=limit,
            score_threshold=min_score,
            query_filter=Filter(must_not=must_not, must=must if must else None),
            with_payload=True,
        )
        return [
            {
                "score": round(point.score, 4),
                "file_path": point.payload.get("file_path"),
                "title": point.payload.get("title"),
                "type": point.payload.get("type"),
                "entity": point.payload.get("entity"),
                "status": point.payload.get("status"),
                "snippet": point.payload.get("content", "")[:200],
            }
            for point in search_results.points
        ]

    def get_indexed_files(self) -> dict[str, str]:
        indexed = {}
        offset = None
        while True:
            points, offset = self._client.scroll(
                collection_name=self._collection,
                scroll_filter=Filter(
                    must=[FieldCondition(key="chunk_index", match=MatchValue(value=0))]
                ),
                limit=100,
                offset=offset,
                with_payload=["file_path", "content_hash"],
            )
            for point in points:
                indexed[point.payload["file_path"]] = point.payload.get("content_hash", "")
            if offset is None:
                break
        return indexed

    def get_stats(self) -> dict:
        info = self._client.get_collection(self._collection)
        return {
            "total_points": info.points_count,
            "status": info.status.value,
            "last_indexed_at": self.last_indexed_at.isoformat() if self.last_indexed_at else None,
        }
