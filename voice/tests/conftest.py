"""Shared test fixtures for voice gateway tests."""

import pytest

from src.config import VoiceConfig


@pytest.fixture
def config() -> VoiceConfig:
    """Provide a test configuration with safe defaults."""
    return VoiceConfig(
        local_ai_url="http://test-localai:8080",
        slack_token="xoxb-test-token",
        anthropic_api_key="sk-ant-test-key",
        voice_auth_token="test-voice-token",
        port=9090,
        whisper_model="whisper-base",
        tts_model="piper",
    )
