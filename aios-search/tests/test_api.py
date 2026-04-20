from unittest.mock import MagicMock, patch

import pytest
from fastapi.testclient import TestClient


@pytest.fixture
def mock_indexer():
    indexer = MagicMock()
    indexer.search.return_value = [
        {
            "score": 0.85,
            "file_path": "20-Meetings/meeting.md",
            "title": "Meeting",
            "type": "meeting",
            "entity": ["diixtra"],
            "status": "done",
            "chunk_index": 0,
            "snippet": "Discussion about IDOX...",
        }
    ]
    indexer.find_similar.return_value = [
        {
            "score": 0.75,
            "file_path": "10-Projects/project.md",
            "title": "Project",
            "type": "project",
            "entity": ["diixtra"],
            "status": "active",
            "snippet": "Related project...",
        }
    ]
    indexer.get_stats.return_value = {
        "total_points": 100,
        "status": "green",
        "last_indexed_at": "2026-03-19T14:00:00+00:00",
    }
    return indexer


@pytest.fixture
def client(mock_indexer, tmp_vault):
    from aios_search.api import create_router
    from fastapi import FastAPI

    app = FastAPI()
    router = create_router(
        indexer=mock_indexer,
        vault_path=str(tmp_vault),
        api_key="test-key",
    )
    app.include_router(router)
    return TestClient(app)


def test_search(client, mock_indexer):
    resp = client.post(
        "/search",
        json={"query": "IDOX migration"},
        headers={"Authorization": "Bearer test-key"},
    )
    assert resp.status_code == 200
    data = resp.json()
    assert len(data["results"]) == 1
    assert data["results"][0]["score"] == 0.85
    mock_indexer.search.assert_called_once()


def test_search_with_filters(client, mock_indexer):
    resp = client.post(
        "/search",
        json={"query": "IDOX", "limit": 3, "filters": {"type": "meeting", "entity": "diixtra"}},
        headers={"Authorization": "Bearer test-key"},
    )
    assert resp.status_code == 200
    call_kwargs = mock_indexer.search.call_args
    assert call_kwargs.kwargs.get("filters") == {"type": "meeting", "entity": "diixtra"}


def test_search_no_auth(client):
    resp = client.post("/search", json={"query": "test"})
    assert resp.status_code == 401


def test_similar(client, mock_indexer):
    resp = client.post(
        "/similar",
        json={"file_path": "20-Meetings/meeting.md"},
        headers={"Authorization": "Bearer test-key"},
    )
    assert resp.status_code == 200
    assert len(resp.json()["results"]) == 1


def test_similar_not_found(client, mock_indexer):
    mock_indexer.find_similar.return_value = None
    resp = client.post(
        "/similar",
        json={"file_path": "nonexistent.md"},
        headers={"Authorization": "Bearer test-key"},
    )
    assert resp.status_code == 404


def test_get_note(client, tmp_vault):
    resp = client.get(
        "/note/12-CRM/Contacts/Shah Ali.md",
        headers={"Authorization": "Bearer test-key"},
    )
    assert resp.status_code == 200
    data = resp.json()
    assert data["title"] == "Shah Ali"
    assert data["metadata"]["type"] == "contact"
    assert "Property sourcing agent" in data["content"]


def test_get_note_not_found(client):
    resp = client.get(
        "/note/nonexistent.md",
        headers={"Authorization": "Bearer test-key"},
    )
    assert resp.status_code == 404


def test_health_no_auth_required(client):
    resp = client.get("/health")
    assert resp.status_code == 200
    assert "total_points" in resp.json()


def test_reindex(client, mock_indexer):
    resp = client.post(
        "/reindex",
        headers={"Authorization": "Bearer test-key"},
    )
    assert resp.status_code == 202
    assert resp.json()["status"] == "reindex_started"


def test_search_vector_db_unavailable(client, mock_indexer):
    mock_indexer.search.side_effect = Exception("Connection refused")
    resp = client.post(
        "/search",
        json={"query": "test"},
        headers={"Authorization": "Bearer test-key"},
    )
    assert resp.status_code == 503
    assert resp.json()["error"] == "Vector database unavailable"


def test_similar_vector_db_unavailable(client, mock_indexer):
    mock_indexer.find_similar.side_effect = Exception("Connection refused")
    resp = client.post(
        "/similar",
        json={"file_path": "test.md"},
        headers={"Authorization": "Bearer test-key"},
    )
    assert resp.status_code == 503
    assert resp.json()["error"] == "Vector database unavailable"
