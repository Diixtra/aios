from aios_search_mcp.config import McpSettings


def test_mcp_config(monkeypatch):
    monkeypatch.setenv("AIOS_SEARCH_URL", "https://aios.example.com")
    monkeypatch.setenv("AIOS_API_KEY", "secret")

    s = McpSettings()
    assert s.aios_search_url == "https://aios.example.com"
    assert s.aios_api_key == "secret"
