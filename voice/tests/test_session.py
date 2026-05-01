"""Tests for voice session management."""

from src.session import VoiceSession, SessionManager


class TestVoiceSession:
    def test_add_user_message(self) -> None:
        session = VoiceSession(task_name="task-1")
        session.add_user_message("Hello agent")

        assert len(session.transcript) == 1
        assert session.transcript[0].role == "user"
        assert session.transcript[0].text == "Hello agent"

    def test_add_agent_message(self) -> None:
        session = VoiceSession(task_name="task-1")
        session.add_agent_message("I can help with that")

        assert len(session.transcript) == 1
        assert session.transcript[0].role == "agent"
        assert session.transcript[0].text == "I can help with that"

    def test_get_transcript_text_formatting(self) -> None:
        session = VoiceSession(task_name="task-1")
        session.add_user_message("What is the status?")
        session.add_agent_message("Task is running.")
        session.add_user_message("Thanks")

        text = session.get_transcript_text()
        lines = text.split("\n")

        assert lines[0] == "User: What is the status?"
        assert lines[1] == "Agent: Task is running."
        assert lines[2] == "User: Thanks"

    def test_get_transcript_text_empty(self) -> None:
        session = VoiceSession(task_name="task-1")
        assert session.get_transcript_text() == ""

    def test_default_values(self) -> None:
        session = VoiceSession(task_name="task-1")
        assert session.task_namespace == "default"
        assert session.transcript == []
        assert session.active is True

    def test_custom_namespace(self) -> None:
        session = VoiceSession(task_name="task-1", task_namespace="production")
        assert session.task_namespace == "production"


class TestSessionManager:
    def test_create_session(self) -> None:
        mgr = SessionManager()
        session = mgr.create("task-1", "ns-1")

        assert session.task_name == "task-1"
        assert session.task_namespace == "ns-1"
        assert session.active is True

    def test_get_existing_session(self) -> None:
        mgr = SessionManager()
        created = mgr.create("task-1")
        retrieved = mgr.get("task-1")

        assert retrieved is created

    def test_get_nonexistent_returns_none(self) -> None:
        mgr = SessionManager()
        assert mgr.get("nonexistent") is None

    def test_close_removes_session(self) -> None:
        mgr = SessionManager()
        session = mgr.create("task-1")
        mgr.close("task-1")

        assert session.active is False
        assert mgr.get("task-1") is None

    def test_close_nonexistent_is_safe(self) -> None:
        mgr = SessionManager()
        mgr.close("nonexistent")  # Should not raise

    def test_create_overwrites_existing(self) -> None:
        mgr = SessionManager()
        first = mgr.create("task-1")
        first.add_user_message("hello")

        second = mgr.create("task-1")
        assert second.transcript == []
        assert mgr.get("task-1") is second
