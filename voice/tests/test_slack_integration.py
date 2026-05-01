"""Tests for Slack voice integration."""

from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from src.slack_integration import SlackVoiceIntegration


class TestSlackVoiceIntegration:
    @pytest.mark.asyncio
    async def test_post_transcript_calls_slack_api(self) -> None:
        mock_client = AsyncMock()
        with patch.object(
            SlackVoiceIntegration,
            "__init__",
            lambda self, token: setattr(self, "client", mock_client),
        ):
            integration = SlackVoiceIntegration(token="xoxb-test-token")

            await integration.post_transcript(
                channel="C123",
                thread_ts="1234.5678",
                transcript="User: Hello\nAgent: Hi there",
            )

            mock_client.chat_postMessage.assert_awaited_once_with(
                channel="C123",
                thread_ts="1234.5678",
                text="*Voice Transcript*\n```\nUser: Hello\nAgent: Hi there\n```",
            )

    @pytest.mark.asyncio
    async def test_post_transcript_formats_message(self) -> None:
        mock_client = AsyncMock()
        with patch.object(
            SlackVoiceIntegration,
            "__init__",
            lambda self, token: setattr(self, "client", mock_client),
        ):
            integration = SlackVoiceIntegration(token="xoxb-test")

            await integration.post_transcript(
                channel="C456",
                thread_ts="9999.0000",
                transcript="User: test",
            )

            call_kwargs = mock_client.chat_postMessage.call_args.kwargs
            assert call_kwargs["text"].startswith("*Voice Transcript*")
            assert "```" in call_kwargs["text"]
            assert "User: test" in call_kwargs["text"]

    def test_init_creates_async_web_client(self) -> None:
        with patch("src.slack_integration.AsyncWebClient") as MockClient:
            mock_instance = MagicMock()
            MockClient.return_value = mock_instance

            integration = SlackVoiceIntegration(token="xoxb-my-token")

            MockClient.assert_called_once_with(token="xoxb-my-token")
            assert integration.client is mock_instance
