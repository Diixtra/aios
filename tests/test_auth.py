import pytest
from fastapi import FastAPI, Depends
from fastapi.testclient import TestClient

from aios_search.auth import require_api_key


def _make_app(api_key: str) -> FastAPI:
    app = FastAPI()

    @app.get("/protected")
    def protected(dep=Depends(require_api_key(api_key))):
        return {"ok": True}

    @app.get("/public")
    def public():
        return {"ok": True}

    return app


def test_valid_api_key():
    client = TestClient(_make_app("secret"))
    resp = client.get("/protected", headers={"Authorization": "Bearer secret"})
    assert resp.status_code == 200


def test_missing_api_key():
    client = TestClient(_make_app("secret"))
    resp = client.get("/protected")
    assert resp.status_code == 401


def test_wrong_api_key():
    client = TestClient(_make_app("secret"))
    resp = client.get("/protected", headers={"Authorization": "Bearer wrong"})
    assert resp.status_code == 401


def test_public_endpoint_no_key():
    client = TestClient(_make_app("secret"))
    resp = client.get("/public")
    assert resp.status_code == 200
