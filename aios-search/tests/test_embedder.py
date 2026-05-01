from unittest.mock import patch, MagicMock

import numpy as np
import pytest

from aios_search.embedder import Embedder


@pytest.fixture
def mock_response():
    """Create a mock httpx response with embedding data."""

    def _make(embeddings: list[list[float]]):
        resp = MagicMock()
        resp.status_code = 200
        resp.raise_for_status = MagicMock()
        resp.json.return_value = {
            "data": [{"embedding": emb, "index": i} for i, emb in enumerate(embeddings)]
        }
        return resp

    return _make


@pytest.fixture
def mock_client(mock_response):
    """Patch httpx.Client to return controlled responses."""
    with patch("aios_search.embedder.httpx.Client") as mock_cls:
        client = MagicMock()
        mock_cls.return_value.__enter__ = MagicMock(return_value=client)
        mock_cls.return_value.__exit__ = MagicMock(return_value=False)
        yield client


def test_embed_single_text(mock_client, mock_response):
    embedding = [0.1] * 384
    mock_client.post.return_value = mock_response([embedding])

    embedder = Embedder(base_url="http://localhost:8080", model_name="all-MiniLM-L6-v2")
    vectors = embedder.embed(["Hello world"])

    assert len(vectors) == 1
    assert len(vectors[0]) == 384
    assert isinstance(vectors[0], np.ndarray)
    mock_client.post.assert_called_once()
    call_kwargs = mock_client.post.call_args
    assert call_kwargs.args[0] == "/v1/embeddings"
    body = call_kwargs.kwargs["json"]
    assert body["model"] == "all-MiniLM-L6-v2"
    assert body["input"] == ["Hello world"]


def test_embed_batch(mock_client, mock_response):
    texts = [f"Text number {i}" for i in range(10)]
    embeddings = [[0.1 * i] * 384 for i in range(10)]
    mock_client.post.return_value = mock_response(embeddings)

    embedder = Embedder(base_url="http://localhost:8080", model_name="all-MiniLM-L6-v2")
    vectors = embedder.embed(texts)

    assert len(vectors) == 10
    assert all(len(v) == 384 for v in vectors)


def test_embed_empty_list(mock_client):
    embedder = Embedder(base_url="http://localhost:8080", model_name="all-MiniLM-L6-v2")
    vectors = embedder.embed([])

    assert vectors == []
    mock_client.post.assert_not_called()


def test_embed_api_error(mock_client):
    import httpx as real_httpx

    resp = MagicMock()
    resp.status_code = 500
    resp.raise_for_status.side_effect = real_httpx.HTTPStatusError(
        "Server Error", request=MagicMock(), response=resp
    )
    mock_client.post.return_value = resp

    embedder = Embedder(base_url="http://localhost:8080", model_name="all-MiniLM-L6-v2")
    with pytest.raises(real_httpx.HTTPStatusError):
        embedder.embed(["test"])
