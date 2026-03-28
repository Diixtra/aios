"""FastAPI voice gateway with WebSocket push-to-talk support."""

from __future__ import annotations

import json
import logging
from pathlib import Path

import httpx
from fastapi import FastAPI, Query, WebSocket, WebSocketDisconnect
from fastapi.responses import FileResponse, JSONResponse

from src.config import VoiceConfig
from src.localai import LocalAIClient
from src.session import SessionManager
from src.slack_integration import SlackVoiceIntegration

logger = logging.getLogger(__name__)

app = FastAPI(title="AIOS Voice Gateway")
config = VoiceConfig.from_env()
localai = LocalAIClient(config)
sessions = SessionManager()

STATIC_DIR = Path(__file__).resolve().parent.parent / "static"


@app.get("/healthz")
async def healthz() -> JSONResponse:
    """Health check endpoint."""
    return JSONResponse({"status": "ok"})


@app.get("/")
async def index() -> FileResponse:
    """Serve the push-to-talk UI."""
    return FileResponse(
        STATIC_DIR / "index.html", media_type="text/html"
    )


async def _get_claude_response(
    task_name: str, transcript: list[dict[str, str]]
) -> str:
    """Call the Anthropic API with the session transcript and return the response.

    Args:
        task_name: The AIOS task name for context.
        transcript: List of message dicts with 'role' and 'content' keys.

    Returns:
        The text content of Claude's response.
    """
    messages = [
        {
            "role": m["role"] if m["role"] == "user" else "assistant",
            "content": m["content"],
        }
        for m in transcript
    ]

    async with httpx.AsyncClient() as client:
        response = await client.post(
            "https://api.anthropic.com/v1/messages",
            headers={
                "x-api-key": config.anthropic_api_key,
                "anthropic-version": "2023-06-01",
                "content-type": "application/json",
            },
            json={
                "model": "claude-sonnet-4-6",
                "max_tokens": 1024,
                "system": (
                    f"You are an AIOS agent assistant discussing task: {task_name}. "
                    "The user is talking to you via voice. Keep responses concise "
                    "and conversational."
                ),
                "messages": messages,
            },
            timeout=30.0,
        )
        response.raise_for_status()
        data = response.json()
        return data["content"][0]["text"]


@app.websocket("/ws/{task_name}")
async def voice_ws(
    websocket: WebSocket,
    task_name: str,
    channel: str = Query(default=""),
    thread: str = Query(default=""),
) -> None:
    """WebSocket endpoint for push-to-talk voice interaction.

    Protocol:
    - Client sends binary audio frames.
    - Server responds with a JSON text message (transcript) followed
      by a binary audio message (TTS synthesis).

    Query Parameters:
        task: The AIOS task name (path param).
        token: Authentication token (required).
        channel: Slack channel ID for transcript posting.
        thread: Slack thread timestamp for transcript posting.
    """
    # Validate auth token from query params
    token = websocket.query_params.get("token")
    if not config.voice_auth_token or not token or token != config.voice_auth_token:
        await websocket.close(code=4001, reason="Unauthorized")
        return

    await websocket.accept()

    session = sessions.get(task_name)
    if session is None:
        session = sessions.create(task_name)

    try:
        while True:
            audio_bytes = await websocket.receive_bytes()

            # Step 1: Transcribe user audio via Whisper
            user_text = await localai.transcribe(audio_bytes)
            session.add_user_message(user_text)

            # Step 2: Generate agent response via Claude API
            transcript_for_claude = [
                {"role": e.role, "content": e.text}
                for e in session.transcript
            ]
            agent_text = await _get_claude_response(
                task_name, transcript_for_claude
            )
            session.add_agent_message(agent_text)

            # Step 3: Send text transcript to client
            await websocket.send_text(
                json.dumps(
                    {
                        "type": "transcript",
                        "user": user_text,
                        "agent": agent_text,
                    }
                )
            )

            # Step 4: Synthesize agent response to audio via Piper
            audio_response = await localai.synthesize(agent_text)
            await websocket.send_bytes(audio_response)

    except WebSocketDisconnect:
        # Post transcript to Slack if we have a non-empty transcript
        # and Slack connection details.
        transcript_text = session.get_transcript_text()
        if transcript_text and channel and thread and config.slack_token:
            try:
                slack = SlackVoiceIntegration(config.slack_token)
                await slack.post_transcript(channel, thread, transcript_text)
            except Exception:
                logger.exception(
                    "Failed to post voice transcript to Slack "
                    "channel=%s thread=%s",
                    channel,
                    thread,
                )

        sessions.close(task_name)
