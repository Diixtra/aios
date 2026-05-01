"""Client for LocalAI Whisper STT and Piper TTS endpoints."""

from __future__ import annotations

import httpx

from src.config import VoiceConfig


class LocalAIClient:
    """Async client for LocalAI audio transcription and synthesis."""

    def __init__(self, config: VoiceConfig) -> None:
        self.base_url = config.local_ai_url.rstrip("/")
        self.whisper_model = config.whisper_model
        self.tts_model = config.tts_model

    async def transcribe(self, audio_bytes: bytes, model: str | None = None) -> str:
        """Transcribe audio bytes to text via LocalAI Whisper endpoint.

        Args:
            audio_bytes: Raw audio data to transcribe.
            model: Whisper model name override (defaults to config value).

        Returns:
            Transcribed text string.
        """
        model = model or self.whisper_model
        async with httpx.AsyncClient() as client:
            response = await client.post(
                f"{self.base_url}/v1/audio/transcriptions",
                files={"file": ("audio.wav", audio_bytes, "audio/wav")},
                data={"model": model},
                timeout=30.0,
            )
            response.raise_for_status()
            data = response.json()
            return data.get("text", "")

    async def synthesize(self, text: str, model: str | None = None) -> bytes:
        """Synthesize text to audio via LocalAI Piper TTS endpoint.

        Args:
            text: Text to convert to speech.
            model: TTS model name override (defaults to config value).

        Returns:
            Audio bytes from the TTS engine.
        """
        model = model or self.tts_model
        async with httpx.AsyncClient() as client:
            response = await client.post(
                f"{self.base_url}/v1/audio/speech",
                json={"input": text, "model": model},
                timeout=30.0,
            )
            response.raise_for_status()
            return response.content
