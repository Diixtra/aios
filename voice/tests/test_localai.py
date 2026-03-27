"""Tests for LocalAI client transcription and synthesis."""

from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from src.config import VoiceConfig
from src.localai import LocalAIClient


@pytest.fixture
def localai_config() -> VoiceConfig:
    return VoiceConfig(
        local_ai_url="http://localai:8080",
        whisper_model="whisper-base",
        tts_model="piper",
    )


@pytest.fixture
def client(localai_config: VoiceConfig) -> LocalAIClient:
    return LocalAIClient(localai_config)


@pytest.mark.asyncio
async def test_transcribe_sends_multipart_and_returns_text(
    client: LocalAIClient,
) -> None:
    mock_response = MagicMock()
    mock_response.json.return_value = {"text": "hello world"}
    mock_response.raise_for_status = MagicMock()

    mock_post = AsyncMock(return_value=mock_response)

    with patch("src.localai.httpx.AsyncClient") as mock_client_cls:
        mock_ctx = AsyncMock()
        mock_ctx.post = mock_post
        mock_client_cls.return_value.__aenter__ = AsyncMock(return_value=mock_ctx)
        mock_client_cls.return_value.__aexit__ = AsyncMock(return_value=None)

        result = await client.transcribe(b"fake-audio-data")

    assert result == "hello world"
    mock_post.assert_called_once()
    call_kwargs = mock_post.call_args
    assert "/v1/audio/transcriptions" in call_kwargs.args[0]
    assert call_kwargs.kwargs["data"]["model"] == "whisper-base"


@pytest.mark.asyncio
async def test_transcribe_with_custom_model(
    client: LocalAIClient,
) -> None:
    mock_response = MagicMock()
    mock_response.json.return_value = {"text": "custom model result"}
    mock_response.raise_for_status = MagicMock()

    mock_post = AsyncMock(return_value=mock_response)

    with patch("src.localai.httpx.AsyncClient") as mock_client_cls:
        mock_ctx = AsyncMock()
        mock_ctx.post = mock_post
        mock_client_cls.return_value.__aenter__ = AsyncMock(return_value=mock_ctx)
        mock_client_cls.return_value.__aexit__ = AsyncMock(return_value=None)

        result = await client.transcribe(b"audio", model="whisper-large")

    assert result == "custom model result"
    call_kwargs = mock_post.call_args
    assert call_kwargs.kwargs["data"]["model"] == "whisper-large"


@pytest.mark.asyncio
async def test_synthesize_sends_json_and_returns_bytes(
    client: LocalAIClient,
) -> None:
    mock_response = MagicMock()
    mock_response.content = b"fake-audio-output"
    mock_response.raise_for_status = MagicMock()

    mock_post = AsyncMock(return_value=mock_response)

    with patch("src.localai.httpx.AsyncClient") as mock_client_cls:
        mock_ctx = AsyncMock()
        mock_ctx.post = mock_post
        mock_client_cls.return_value.__aenter__ = AsyncMock(return_value=mock_ctx)
        mock_client_cls.return_value.__aexit__ = AsyncMock(return_value=None)

        result = await client.synthesize("hello world")

    assert result == b"fake-audio-output"
    mock_post.assert_called_once()
    call_kwargs = mock_post.call_args
    assert "/v1/audio/speech" in call_kwargs.args[0]
    assert call_kwargs.kwargs["json"] == {"input": "hello world", "model": "piper"}


@pytest.mark.asyncio
async def test_synthesize_with_custom_model(
    client: LocalAIClient,
) -> None:
    mock_response = MagicMock()
    mock_response.content = b"custom-tts-audio"
    mock_response.raise_for_status = MagicMock()

    mock_post = AsyncMock(return_value=mock_response)

    with patch("src.localai.httpx.AsyncClient") as mock_client_cls:
        mock_ctx = AsyncMock()
        mock_ctx.post = mock_post
        mock_client_cls.return_value.__aenter__ = AsyncMock(return_value=mock_ctx)
        mock_client_cls.return_value.__aexit__ = AsyncMock(return_value=None)

        result = await client.synthesize("test", model="custom-tts")

    assert result == b"custom-tts-audio"
    call_kwargs = mock_post.call_args
    assert call_kwargs.kwargs["json"] == {"input": "test", "model": "custom-tts"}
