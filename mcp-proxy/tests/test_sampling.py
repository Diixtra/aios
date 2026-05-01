"""Tests for mcp_sampling_proxy.sampling parsing logic."""

from __future__ import annotations

import json
from unittest import mock

import pytest

import mcp.types as mcp_types

from mcp_sampling_proxy.config import Config
from mcp_sampling_proxy.sampling import SamplingExecutor, _STOP_REASON_MAP


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_config(**overrides: object) -> Config:
    defaults = {"upstream_url": "http://upstream:8080", "claude_path": "claude"}
    return Config(**{**defaults, **overrides})


def _stream_json_line(
    msg_type: str,
    content: list[dict],
    stop_reason: str = "end_turn",
    model: str = "claude-test",
) -> str:
    return json.dumps(
        {
            "type": msg_type,
            "message": {
                "role": "assistant",
                "content": content,
                "stop_reason": stop_reason,
                "model": model,
            },
        }
    )


def _make_params(
    text: str = "hello", system: str | None = None
) -> mcp_types.CreateMessageRequestParams:
    return mcp_types.CreateMessageRequestParams(
        messages=[
            mcp_types.SamplingMessage(
                role="user",
                content=mcp_types.TextContent(type="text", text=text),
            )
        ],
        maxTokens=1024,
        systemPrompt=system,
    )


def _mock_process(stdout: str, returncode: int = 0, stderr: str = "") -> mock.AsyncMock:
    proc = mock.AsyncMock()
    proc.communicate.return_value = (stdout.encode(), stderr.encode())
    proc.returncode = returncode
    proc.terminate = mock.Mock()
    proc.kill = mock.Mock()
    proc.wait = mock.AsyncMock()
    return proc


# ---------------------------------------------------------------------------
# Tests: stop-reason mapping
# ---------------------------------------------------------------------------


class TestStopReasonMap:
    def test_all_known_mappings(self) -> None:
        assert _STOP_REASON_MAP["end_turn"] == "endTurn"
        assert _STOP_REASON_MAP["tool_use"] == "toolUse"
        assert _STOP_REASON_MAP["max_tokens"] == "maxTokens"
        assert _STOP_REASON_MAP["stop_sequence"] == "stopSequence"


# ---------------------------------------------------------------------------
# Tests: SamplingExecutor.execute
# ---------------------------------------------------------------------------


class TestSamplingExecutor:
    @pytest.mark.asyncio
    async def test_simple_text_response(self) -> None:
        stdout = _stream_json_line(
            "assistant", [{"type": "text", "text": "Hello world"}]
        )
        proc = _mock_process(stdout)

        with mock.patch("asyncio.create_subprocess_exec", return_value=proc):
            executor = SamplingExecutor(_make_config())
            result = await executor.execute(_make_params("hi"))

        assert result.role == "assistant"
        assert result.content.text == "Hello world"
        assert result.stopReason == "endTurn"
        assert result.model == "claude-test"

    @pytest.mark.asyncio
    async def test_uses_last_assistant_message(self) -> None:
        """After Issue 11 fix: should use LAST assistant message, not first."""
        first = _stream_json_line(
            "assistant", [{"type": "text", "text": "first response"}]
        )
        second = _stream_json_line(
            "assistant", [{"type": "text", "text": "final response"}]
        )
        stdout = first + "\n" + second
        proc = _mock_process(stdout)

        with mock.patch("asyncio.create_subprocess_exec", return_value=proc):
            executor = SamplingExecutor(_make_config())
            result = await executor.execute(_make_params("hi"))

        assert result.content.text == "final response"

    @pytest.mark.asyncio
    async def test_tool_use_response(self) -> None:
        tool_block = {
            "type": "tool_use",
            "id": "t1",
            "name": "bash",
            "input": {"cmd": "ls"},
        }
        stdout = _stream_json_line("assistant", [tool_block], stop_reason="tool_use")
        proc = _mock_process(stdout)

        with mock.patch("asyncio.create_subprocess_exec", return_value=proc):
            executor = SamplingExecutor(_make_config())
            result = await executor.execute(_make_params("run ls"))

        assert result.stopReason == "toolUse"
        parsed = json.loads(result.content.text)
        assert "tool_use" in parsed

    @pytest.mark.asyncio
    async def test_nonzero_exit_raises(self) -> None:
        proc = _mock_process("", returncode=1, stderr="fatal error")

        with mock.patch("asyncio.create_subprocess_exec", return_value=proc):
            executor = SamplingExecutor(_make_config())
            with pytest.raises(Exception, match="fatal error"):
                await executor.execute(_make_params("hi"))

    @pytest.mark.asyncio
    async def test_no_assistant_message_raises(self) -> None:
        stdout = json.dumps({"type": "system", "message": {}})
        proc = _mock_process(stdout)

        with mock.patch("asyncio.create_subprocess_exec", return_value=proc):
            executor = SamplingExecutor(_make_config())
            with pytest.raises(Exception, match="No assistant message"):
                await executor.execute(_make_params("hi"))

    @pytest.mark.asyncio
    async def test_system_prompt_passed_to_args(self) -> None:
        stdout = _stream_json_line("assistant", [{"type": "text", "text": "ok"}])
        proc = _mock_process(stdout)

        with mock.patch(
            "asyncio.create_subprocess_exec", return_value=proc
        ) as mock_exec:
            executor = SamplingExecutor(_make_config())
            await executor.execute(_make_params("hi", system="Be helpful"))

        call_args = mock_exec.call_args[0]
        assert "--system-prompt" in call_args
        idx = call_args.index("--system-prompt")
        assert call_args[idx + 1] == "Be helpful"

    @pytest.mark.asyncio
    async def test_binary_not_found_raises(self) -> None:
        with mock.patch(
            "asyncio.create_subprocess_exec",
            side_effect=FileNotFoundError("not found"),
        ):
            executor = SamplingExecutor(_make_config())
            with pytest.raises(Exception, match="claude binary not found"):
                await executor.execute(_make_params("hi"))

    @pytest.mark.asyncio
    async def test_multiple_text_blocks_joined(self) -> None:
        stdout = _stream_json_line(
            "assistant",
            [
                {"type": "text", "text": "Hello"},
                {"type": "text", "text": "World"},
            ],
        )
        proc = _mock_process(stdout)

        with mock.patch("asyncio.create_subprocess_exec", return_value=proc):
            executor = SamplingExecutor(_make_config())
            result = await executor.execute(_make_params("hi"))

        assert result.content.text == "Hello World"
