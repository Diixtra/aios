"""Slack integration for posting voice session transcripts."""

from __future__ import annotations

from slack_sdk.web.async_client import AsyncWebClient


class SlackVoiceIntegration:
    """Posts voice conversation transcripts to Slack threads."""

    def __init__(self, token: str) -> None:
        self.client = AsyncWebClient(token=token)

    async def post_transcript(
        self, channel: str, thread_ts: str, transcript: str
    ) -> None:
        """Post a voice transcript to a Slack thread.

        Args:
            channel: Slack channel ID.
            thread_ts: Thread timestamp to reply to.
            transcript: Formatted transcript text.
        """
        await self.client.chat_postMessage(
            channel=channel,
            thread_ts=thread_ts,
            text=f"*Voice Transcript*\n```\n{transcript}\n```",
        )
