"""Tests for mcp_sampling_proxy.config."""

from __future__ import annotations

import os
from unittest import mock

import pytest

from mcp_sampling_proxy.config import Config, load_config


class TestConfig:
    """Tests for the Config dataclass."""

    def test_defaults(self) -> None:
        cfg = Config(upstream_url="http://localhost:8080")
        assert cfg.upstream_url == "http://localhost:8080"
        assert cfg.claude_path == "claude"
        assert cfg.sampling_timeout_s == 120
        assert cfg.debug is False

    def test_frozen(self) -> None:
        cfg = Config(upstream_url="http://localhost:8080")
        with pytest.raises(AttributeError):
            cfg.upstream_url = "http://other"  # type: ignore[misc]


class TestLoadConfig:
    """Tests for load_config() env-var + arg parsing."""

    def test_loads_from_env(self) -> None:
        env = {
            "UPSTREAM_URL": "http://upstream:9090",
            "CLAUDE_PATH": "/usr/bin/claude",
            "SAMPLING_TIMEOUT_S": "60",
            "DEBUG": "true",
        }
        with (
            mock.patch.dict(os.environ, env, clear=False),
            mock.patch("sys.argv", ["prog"]),
        ):
            cfg = load_config()
        assert cfg.upstream_url == "http://upstream:9090"
        assert cfg.claude_path == "/usr/bin/claude"
        assert cfg.sampling_timeout_s == 60
        assert cfg.debug is True

    def test_missing_upstream_url_exits(self) -> None:
        with (
            mock.patch.dict(os.environ, {}, clear=True),
            mock.patch("sys.argv", ["prog"]),
            pytest.raises(SystemExit),
        ):
            load_config()

    def test_cli_args_override_env(self) -> None:
        env = {"UPSTREAM_URL": "http://from-env:1234"}
        with (
            mock.patch.dict(os.environ, env, clear=False),
            mock.patch("sys.argv", ["prog", "--upstream-url", "http://from-arg:5678"]),
        ):
            cfg = load_config()
        assert cfg.upstream_url == "http://from-arg:5678"

    def test_debug_flag_from_env_value_1(self) -> None:
        env = {"UPSTREAM_URL": "http://x", "DEBUG": "1"}
        with (
            mock.patch.dict(os.environ, env, clear=False),
            mock.patch("sys.argv", ["prog"]),
        ):
            cfg = load_config()
        assert cfg.debug is True

    def test_debug_flag_off_by_default(self) -> None:
        env = {"UPSTREAM_URL": "http://x"}
        with (
            mock.patch.dict(os.environ, env, clear=False),
            mock.patch("sys.argv", ["prog"]),
        ):
            cfg = load_config()
        assert cfg.debug is False
