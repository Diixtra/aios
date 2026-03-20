import uuid
from unittest.mock import MagicMock, patch

import numpy as np
import pytest

from aios_search.indexer import Indexer
from aios_search.parser import NoteChunk


@pytest.fixture
def mock_qdrant():
    with patch("aios_search.indexer.QdrantClient") as mock_cls:
        client = MagicMock()
        mock_cls.return_value = client
        yield client


@pytest.fixture
def mock_embedder():
    embedder = MagicMock()
    embedder.embed.return_value = [np.random.rand(384).astype(np.float32)]
    return embedder


@pytest.fixture
def sample_chunks():
    return [
        NoteChunk(
            file_path="12-CRM/Contacts/Shah Ali.md",
            title="Shah Ali",
            metadata={"type": "contact", "entity": ["properties"], "status": "active"},
            content="Title: Shah Ali. Type: contact.\n\nProperty sourcing agent.",
            content_hash="abc123",
            chunk_index=0,
            chunk_total=1,
        )
    ]


def test_indexer_ensure_collection(mock_qdrant, mock_embedder):
    mock_qdrant.collection_exists.return_value = False
    indexer = Indexer(
        qdrant_url="http://localhost:6333",
        qdrant_api_key="test",
        collection_name="test_coll",
        vector_size=384,
        embedder=mock_embedder,
    )
    indexer.ensure_collection()
    mock_qdrant.create_collection.assert_called_once()


def test_indexer_upsert_chunks(mock_qdrant, mock_embedder, sample_chunks):
    mock_embedder.embed.return_value = [np.random.rand(384).astype(np.float32)]
    indexer = Indexer(
        qdrant_url="http://localhost:6333",
        qdrant_api_key="test",
        collection_name="test_coll",
        vector_size=384,
        embedder=mock_embedder,
    )
    indexer.upsert_chunks(sample_chunks)
    mock_embedder.embed.assert_called_once()
    mock_qdrant.upsert.assert_called_once()


def test_indexer_delete_by_file_path(mock_qdrant, mock_embedder):
    indexer = Indexer(
        qdrant_url="http://localhost:6333",
        qdrant_api_key="test",
        collection_name="test_coll",
        vector_size=384,
        embedder=mock_embedder,
    )
    indexer.delete_by_file_path("12-CRM/Contacts/Shah Ali.md")
    mock_qdrant.delete.assert_called_once()


def test_indexer_search(mock_qdrant, mock_embedder):
    mock_qdrant.query_points.return_value = MagicMock(points=[])
    mock_embedder.embed.return_value = [np.random.rand(384).astype(np.float32)]
    indexer = Indexer(
        qdrant_url="http://localhost:6333",
        qdrant_api_key="test",
        collection_name="test_coll",
        vector_size=384,
        embedder=mock_embedder,
    )
    results = indexer.search("test query", limit=5, min_score=0.3)
    mock_embedder.embed.assert_called_once_with(["test query"])
    mock_qdrant.query_points.assert_called_once()


def test_indexer_point_id_is_deterministic(mock_qdrant, mock_embedder):
    from aios_search.indexer import make_point_id
    id1 = make_point_id("12-CRM/Contacts/Shah Ali.md", 0)
    id2 = make_point_id("12-CRM/Contacts/Shah Ali.md", 0)
    id3 = make_point_id("12-CRM/Contacts/Shah Ali.md", 1)
    assert id1 == id2
    assert id1 != id3
    assert isinstance(id1, str)
    uuid.UUID(id1)


def test_indexer_get_indexed_file_paths(mock_qdrant, mock_embedder):
    mock_qdrant.scroll.return_value = (
        [
            MagicMock(payload={"file_path": "a.md", "content_hash": "h1"}),
            MagicMock(payload={"file_path": "b.md", "content_hash": "h2"}),
        ],
        None,
    )
    indexer = Indexer(
        qdrant_url="http://localhost:6333",
        qdrant_api_key="test",
        collection_name="test_coll",
        vector_size=384,
        embedder=mock_embedder,
    )
    result = indexer.get_indexed_files()
    assert result == {"a.md": "h1", "b.md": "h2"}
