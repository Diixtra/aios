"""Voice session management for tracking conversations with agents."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Literal


@dataclass
class TranscriptEntry:
    """A single entry in a voice conversation transcript."""

    role: Literal["user", "agent"]
    text: str


@dataclass
class VoiceSession:
    """Tracks a voice conversation with an AIOS agent task."""

    task_name: str
    task_namespace: str = "default"
    transcript: list[TranscriptEntry] = field(default_factory=list)
    active: bool = True

    def add_user_message(self, text: str) -> None:
        """Append a user message to the transcript."""
        self.transcript.append(TranscriptEntry(role="user", text=text))

    def add_agent_message(self, text: str) -> None:
        """Append an agent response to the transcript."""
        self.transcript.append(TranscriptEntry(role="agent", text=text))

    def get_transcript_text(self) -> str:
        """Return the full transcript as formatted text."""
        lines: list[str] = []
        for entry in self.transcript:
            prefix = "User" if entry.role == "user" else "Agent"
            lines.append(f"{prefix}: {entry.text}")
        return "\n".join(lines)


class SessionManager:
    """Manages voice sessions keyed by task name."""

    def __init__(self) -> None:
        self._sessions: dict[str, VoiceSession] = {}

    def create(self, task_name: str, task_namespace: str = "default") -> VoiceSession:
        """Create a new voice session for a task."""
        session = VoiceSession(task_name=task_name, task_namespace=task_namespace)
        self._sessions[task_name] = session
        return session

    def get(self, task_name: str) -> VoiceSession | None:
        """Retrieve an existing session by task name."""
        return self._sessions.get(task_name)

    def close(self, task_name: str) -> None:
        """Close and remove a session."""
        session = self._sessions.pop(task_name, None)
        if session is not None:
            session.active = False
