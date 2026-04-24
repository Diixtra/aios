import threading
import time
from unittest.mock import MagicMock

import pytest
from fastapi.testclient import TestClient


@pytest.fixture
def app_factory(monkeypatch, tmp_vault):
    from aios_search.config import Settings
    from aios_search.main import create_app

    monkeypatch.setenv("VAULT_PATH", str(tmp_vault))
    monkeypatch.setenv("DATABASE_URL", "postgresql://user:pass@localhost:5432/aios")
    monkeypatch.setenv("AIOS_API_KEY", "secret")

    created = {}

    def factory(indexer=None, watcher=None):
        settings = Settings()
        app = create_app(
            settings=settings,
            indexer_override=indexer,
            watcher_override=watcher,
        )
        created["app"] = app
        return app

    return factory


@pytest.fixture
def fake_indexer():
    indexer = MagicMock()
    indexer.get_stats.return_value = {
        "total_points": 42,
        "status": "green",
        "last_indexed_at": None,
    }
    indexer.get_indexed_files.return_value = {}
    return indexer


class _FakeWatcher:
    def __init__(self, reconcile_block_seconds: float = 0.0, reconcile_raises: Exception | None = None):
        self._reconcile_block_seconds = reconcile_block_seconds
        self._reconcile_raises = reconcile_raises
        self.started = False
        self.stopped = False
        self.reconcile_called = threading.Event()
        self.reconcile_finished = threading.Event()

    def reconcile(self):
        self.reconcile_called.set()
        if self._reconcile_block_seconds:
            time.sleep(self._reconcile_block_seconds)
        if self._reconcile_raises:
            raise self._reconcile_raises
        self.reconcile_finished.set()

    def start(self):
        self.started = True

    def stop(self):
        self.stopped = True

    def full_reindex(self):
        pass


def test_lifespan_does_not_block_on_slow_reconcile(app_factory, fake_indexer):
    """
    Regression for #1425: lifespan must not wait for reconcile to complete.
    If reconcile blocks (e.g. embeddings backend down), /health must still bind.
    """
    watcher = _FakeWatcher(reconcile_block_seconds=5.0)
    app = app_factory(indexer=fake_indexer, watcher=watcher)

    start = time.monotonic()
    with TestClient(app) as client:
        elapsed = time.monotonic() - start
        # TestClient completes lifespan startup before returning from __enter__.
        # If reconcile were awaited synchronously, this would take ~5s.
        assert elapsed < 2.0, f"Lifespan took {elapsed:.2f}s; reconcile is blocking startup"
        resp = client.get("/health")
        assert resp.status_code == 200


def test_lifespan_starts_reconcile_in_background(app_factory, fake_indexer):
    watcher = _FakeWatcher()
    app = app_factory(indexer=fake_indexer, watcher=watcher)

    with TestClient(app):
        # Reconcile runs in a background task; it should be dispatched by now.
        assert watcher.reconcile_called.wait(timeout=5.0)
        assert watcher.reconcile_finished.wait(timeout=5.0)


def test_lifespan_tolerates_reconcile_failure(app_factory, fake_indexer):
    """
    AC1: pod reaches Ready even when embeddings backend is down.
    The background reconcile may raise; the app must keep serving.
    """
    watcher = _FakeWatcher(reconcile_raises=RuntimeError("embeddings backend 500"))
    app = app_factory(indexer=fake_indexer, watcher=watcher)

    with TestClient(app) as client:
        assert watcher.reconcile_called.wait(timeout=5.0)
        # Give the background task a moment to record the error
        time.sleep(0.2)
        resp = client.get("/health")
        assert resp.status_code == 200
        body = resp.json()
        assert body["reconcile"]["stage"] == "error"
        assert "embeddings backend 500" in body["reconcile"]["error"]


def test_lifespan_tolerates_ensure_collection_failure(app_factory, fake_indexer):
    fake_indexer.ensure_collection.side_effect = RuntimeError("qdrant unreachable")
    watcher = _FakeWatcher()
    app = app_factory(indexer=fake_indexer, watcher=watcher)

    with TestClient(app) as client:
        time.sleep(0.2)
        resp = client.get("/health")
        assert resp.status_code == 200
        body = resp.json()
        assert body["reconcile"]["stage"] == "error"


def test_lifespan_stops_watcher_on_shutdown(app_factory, fake_indexer):
    watcher = _FakeWatcher()
    app = app_factory(indexer=fake_indexer, watcher=watcher)
    with TestClient(app):
        pass
    assert watcher.stopped
