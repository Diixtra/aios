"""Tests for the voice gateway HTTP and WebSocket endpoints."""

from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from httpx import ASGITransport, AsyncClient
from starlette.testclient import TestClient

from src.gateway import _get_claude_response, app, config


@pytest.mark.asyncio
async def test_healthz_returns_ok() -> None:
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        response = await client.get("/healthz")
    assert response.status_code == 200
    assert response.json() == {"status": "ok"}


@pytest.mark.asyncio
async def test_index_returns_html() -> None:
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        response = await client.get("/")
    assert response.status_code == 200
    assert "voice" in response.text.lower()


@pytest.mark.asyncio
async def test_get_claude_response_calls_api() -> None:
    """Test that _get_claude_response calls the Anthropic API correctly."""
    mock_response = MagicMock()
    mock_response.status_code = 200
    mock_response.raise_for_status = MagicMock()
    mock_response.json.return_value = {
        "content": [{"type": "text", "text": "Hello from Claude"}],
    }

    mock_client_instance = AsyncMock()
    mock_client_instance.post.return_value = mock_response
    mock_client_instance.__aenter__ = AsyncMock(
        return_value=mock_client_instance
    )
    mock_client_instance.__aexit__ = AsyncMock(return_value=False)

    with patch("src.gateway.httpx.AsyncClient", return_value=mock_client_instance):
        result = await _get_claude_response(
            "test-task",
            [{"role": "user", "content": "Hello"}],
        )

    assert result == "Hello from Claude"
    mock_client_instance.post.assert_called_once()
    call_kwargs = mock_client_instance.post.call_args
    assert call_kwargs[0][0] == "https://api.anthropic.com/v1/messages"
    body = call_kwargs[1]["json"]
    assert body["model"] == "claude-sonnet-4-6"
    assert body["max_tokens"] == 1024
    assert "test-task" in body["system"]
    assert body["messages"] == [{"role": "user", "content": "Hello"}]


@pytest.mark.asyncio
async def test_get_claude_response_maps_agent_to_assistant() -> None:
    """Test that agent role is mapped to assistant for Claude API."""
    mock_response = MagicMock()
    mock_response.status_code = 200
    mock_response.raise_for_status = MagicMock()
    mock_response.json.return_value = {
        "content": [{"type": "text", "text": "Response"}],
    }

    mock_client_instance = AsyncMock()
    mock_client_instance.post.return_value = mock_response
    mock_client_instance.__aenter__ = AsyncMock(
        return_value=mock_client_instance
    )
    mock_client_instance.__aexit__ = AsyncMock(return_value=False)

    with patch("src.gateway.httpx.AsyncClient", return_value=mock_client_instance):
        await _get_claude_response(
            "test-task",
            [
                {"role": "user", "content": "Hi"},
                {"role": "agent", "content": "Hello"},
                {"role": "user", "content": "How are you?"},
            ],
        )

    call_kwargs = mock_client_instance.post.call_args
    messages = call_kwargs[1]["json"]["messages"]
    assert messages[0]["role"] == "user"
    assert messages[1]["role"] == "assistant"
    assert messages[2]["role"] == "user"


def test_ws_rejects_missing_token() -> None:
    """WebSocket connection without token is rejected with 4001."""
    from src.config import VoiceConfig

    fake_config = VoiceConfig(voice_auth_token="secret-token")
    client = TestClient(app)
    with patch("src.gateway.config", fake_config):
        with pytest.raises(Exception):
            with client.websocket_connect("/ws/test-task"):
                pass  # pragma: no cover


def test_ws_rejects_invalid_token() -> None:
    """WebSocket connection with wrong token is rejected with 4001."""
    from src.config import VoiceConfig

    fake_config = VoiceConfig(voice_auth_token="secret-token")
    client = TestClient(app)
    with patch("src.gateway.config", fake_config):
        with pytest.raises(Exception):
            with client.websocket_connect("/ws/test-task?token=wrong"):
                pass  # pragma: no cover


def test_ws_accepts_valid_token() -> None:
    """WebSocket connection with correct token is accepted."""
    from src.config import VoiceConfig

    fake_config = VoiceConfig(voice_auth_token="secret-token")
    client = TestClient(app)
    with patch("src.gateway.config", fake_config):
        with client.websocket_connect("/ws/test-task?token=secret-token") as ws:
            # Connection accepted; close cleanly
            pass
