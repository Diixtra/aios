import os

import pytest


def test_config_loads_from_env(monkeypatch):
    monkeypatch.setenv("VAULT_PATH", "/data/vault")
    monkeypatch.setenv("QDRANT_URL", "https://qdrant.example.com")
    monkeypatch.setenv("QDRANT_API_KEY", "test-key")
    monkeypatch.setenv("AIOS_API_KEY", "secret")

    from aios_search.config import Settings

    s = Settings()
    assert s.vault_path == "/data/vault"
    assert s.qdrant_url == "https://qdrant.example.com"
    assert s.qdrant_api_key == "test-key"
    assert s.aios_api_key == "secret"
    assert s.collection_name == "aios_vault"
    assert s.embedding_model == "all-MiniLM-L6-v2"
    assert s.embedding_url == "http://local-ai.local-ai.svc.cluster.local:8080"
    assert s.min_score == 0.3
    assert s.qdrant_batch_size == 100
    assert s.debounce_ms == 500


def test_config_custom_embedding_url(monkeypatch):
    monkeypatch.setenv("VAULT_PATH", "/data/vault")
    monkeypatch.setenv("QDRANT_URL", "https://qdrant.example.com")
    monkeypatch.setenv("QDRANT_API_KEY", "test-key")
    monkeypatch.setenv("AIOS_API_KEY", "secret")
    monkeypatch.setenv("EMBEDDING_URL", "https://local-ai.lab.kazie.co.uk")

    from aios_search.config import Settings

    s = Settings()
    assert s.embedding_url == "https://local-ai.lab.kazie.co.uk"


def test_config_defaults(monkeypatch):
    monkeypatch.setenv("VAULT_PATH", "/data/vault")
    monkeypatch.setenv("QDRANT_URL", "https://qdrant.example.com")
    monkeypatch.setenv("QDRANT_API_KEY", "test-key")
    monkeypatch.setenv("AIOS_API_KEY", "secret")

    from aios_search.config import Settings

    s = Settings()
    assert s.host == "0.0.0.0"
    assert s.port == 8000


def test_config_ignored_paths(monkeypatch):
    monkeypatch.setenv("VAULT_PATH", "/data/vault")
    monkeypatch.setenv("QDRANT_URL", "https://qdrant.example.com")
    monkeypatch.setenv("QDRANT_API_KEY", "test-key")
    monkeypatch.setenv("AIOS_API_KEY", "secret")

    from aios_search.config import Settings

    s = Settings()
    assert ".obsidian" in s.ignored_dirs
    assert "80-Dashboards" in s.ignored_dirs
    assert "90-Templates" in s.ignored_dirs
