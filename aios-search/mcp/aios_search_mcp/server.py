import httpx
from mcp.server.fastmcp import FastMCP

from aios_search_mcp.config import McpSettings

settings = McpSettings()
mcp = FastMCP("aios-search")


def _client() -> httpx.Client:
    return httpx.Client(
        base_url=settings.aios_search_url,
        headers={"Authorization": f"Bearer {settings.aios_api_key}"},
        timeout=30.0,
    )


def _handle_error(resp: httpx.Response) -> dict:
    if resp.status_code >= 400:
        return {"error": resp.json().get("detail", resp.text)}
    return resp.json()


@mcp.tool()
def semantic_search(
    query: str,
    limit: int = 5,
    type: str | None = None,
    entity: str | None = None,
    status: str | None = None,
) -> dict:
    """Search the AIOS vault by meaning. Returns notes semantically similar to the query.

    Args:
        query: Natural language search query
        limit: Max results to return (default 5)
        type: Filter by note type (e.g., meeting, contact, project, deal)
        entity: Filter by entity (e.g., diixtra, properties, capital, media, group)
        status: Filter by status (e.g., active, done, draft)
    """
    filters = {}
    if type:
        filters["type"] = type
    if entity:
        filters["entity"] = entity
    if status:
        filters["status"] = status

    with _client() as client:
        resp = client.post(
            "/search",
            json={"query": query, "limit": limit, "filters": filters or None},
        )
        return _handle_error(resp)


@mcp.tool()
def get_note(file_path: str) -> dict:
    """Retrieve the full content and metadata of a specific vault note.

    Args:
        file_path: Relative path to the note (e.g., '12-CRM/Contacts/Shah Ali.md')
    """
    with _client() as client:
        resp = client.get(f"/note/{file_path}")
        return _handle_error(resp)


@mcp.tool()
def find_similar(
    file_path: str,
    limit: int = 5,
    type: str | None = None,
    entity: str | None = None,
    status: str | None = None,
) -> dict:
    """Find notes similar to a given note.

    Args:
        file_path: Relative path to the source note
        limit: Max results to return (default 5)
        type: Filter by note type
        entity: Filter by entity
        status: Filter by status
    """
    filters = {}
    if type:
        filters["type"] = type
    if entity:
        filters["entity"] = entity
    if status:
        filters["status"] = status

    with _client() as client:
        resp = client.post(
            "/similar",
            json={"file_path": file_path, "limit": limit, "filters": filters or None},
        )
        return _handle_error(resp)


@mcp.tool()
def vault_stats() -> dict:
    """Get AIOS vault search index health and statistics."""
    with _client() as client:
        resp = client.get("/health")
        return _handle_error(resp)


if __name__ == "__main__":
    mcp.run()
