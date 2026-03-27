"""FastAPI voice gateway with WebSocket push-to-talk support."""

from __future__ import annotations

import json
from pathlib import Path

from fastapi import FastAPI, WebSocket, WebSocketDisconnect
from fastapi.responses import FileResponse, JSONResponse

from src.config import VoiceConfig
from src.localai import LocalAIClient
from src.session import SessionManager

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


@app.websocket("/ws/{task_name}")
async def voice_ws(websocket: WebSocket, task_name: str) -> None:
    """WebSocket endpoint for push-to-talk voice interaction.

    Protocol:
    - Client sends binary audio frames.
    - Server responds with a JSON text message (transcript) followed
      by a binary audio message (TTS synthesis).
    """
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

            # Step 2: Generate agent response (placeholder - will be
            # replaced with Claude API call)
            agent_text = f"I heard you say: {user_text}"
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
        sessions.close(task_name)
