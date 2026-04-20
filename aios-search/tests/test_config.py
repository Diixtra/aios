import pytest


@pytest.fixture(autouse=True)
def _clear_known_env(monkeypatch):
    # Start each test from a clean slate so earlier-test env doesn't leak.
    for var in (
        "VAULT_PATH",
        "DATABASE_URL",
        "AIOS_API_KEY",
        "EMBEDDING_URL",
        "EMBEDDING_MODEL",
        "VECTOR_SIZE",
        "MIN_SCORE",
        "UPSERT_BATCH_SIZE",
        "DEBOUNCE_MS",
        "HOST",
        "PORT",
    ):
        monkeypatch.delenv(var, raising=False)
    yield


def _set_required(monkeypatch):
    monkeypatch.setenv("VAULT_PATH", "/data/vault")
    monkeypatch.setenv(
        "DATABASE_URL",
        "postgresql://aios_search:secret@cnpg-aios-search-rw.ai.svc:5432/aios_search",
    )
    monkeypatch.setenv("AIOS_API_KEY", "secret")


def test_config_loads_from_env(monkeypatch):
    _set_required(monkeypatch)
    from aios_search.config import Settings

    s = Settings()
    assert s.vault_path == "/data/vault"
    assert s.database_url.startswith("postgresql://")
    assert s.aios_api_key == "secret"
    assert s.embedding_model == "all-MiniLM-L6-v2"
    assert s.embedding_url == "http://local-ai.local-ai.svc.cluster.local:8080"
    assert s.vector_size == 384
    assert s.min_score == 0.3
    assert s.upsert_batch_size == 100
    assert s.debounce_ms == 500


def test_config_custom_embedding_url(monkeypatch):
    _set_required(monkeypatch)
    monkeypatch.setenv("EMBEDDING_URL", "https://local-ai.lab.kazie.co.uk")
    from aios_search.config import Settings

    s = Settings()
    assert s.embedding_url == "https://local-ai.lab.kazie.co.uk"


def test_config_defaults(monkeypatch):
    _set_required(monkeypatch)
    from aios_search.config import Settings

    s = Settings()
    assert s.host == "0.0.0.0"
    assert s.port == 8000


def test_config_ignored_paths(monkeypatch):
    _set_required(monkeypatch)
    from aios_search.config import Settings

    s = Settings()
    assert ".obsidian" in s.ignored_dirs
    assert "80-Dashboards" in s.ignored_dirs
    assert "90-Templates" in s.ignored_dirs


def test_config_missing_database_url_fails(monkeypatch):
    monkeypatch.setenv("VAULT_PATH", "/data/vault")
    monkeypatch.setenv("AIOS_API_KEY", "secret")
    from aios_search.config import Settings

    with pytest.raises(Exception):  # pydantic.ValidationError
        Settings()
