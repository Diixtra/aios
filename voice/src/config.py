"""Voice gateway configuration loaded from environment variables."""

from __future__ import annotations

import os
from dataclasses import dataclass, field


@dataclass(frozen=True)
class VoiceConfig:
    """Configuration for the AIOS Voice Gateway."""

    local_ai_url: str = "http://localhost:8080"
    slack_token: str = ""
    anthropic_api_key: str = ""
    port: int = 8080
    whisper_model: str = "whisper-base"
    tts_model: str = "piper"

    @classmethod
    def from_env(cls) -> VoiceConfig:
        """Load configuration from environment variables."""
        return cls(
            local_ai_url=os.getenv("LOCAL_AI_URL", "http://localhost:8080"),
            slack_token=os.getenv("SLACK_TOKEN", ""),
            anthropic_api_key=os.getenv("ANTHROPIC_API_KEY", ""),
            port=int(os.getenv("PORT", "8080")),
            whisper_model=os.getenv("WHISPER_MODEL", "whisper-base"),
            tts_model=os.getenv("TTS_MODEL", "piper"),
        )
