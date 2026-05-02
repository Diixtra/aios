---
title: "Add LOG_LEVEL env var support to the webhook service"
slug: 003-add-go-env-var
difficulty: small
agentType: code-pr
---

The `webhook` service currently logs at the default `slog` level (Info). Add support for a `LOG_LEVEL` environment variable that overrides this.

Requirements:

- Read `LOG_LEVEL` from the environment at startup. Accept the standard slog level names case-insensitively: `debug`, `info`, `warn`, `error`.
- If unset or empty, default to `info`.
- If set to an unrecognised value, log a warning at info level (`"unknown LOG_LEVEL %q, defaulting to info"`) and use info.
- Configure the slog default logger before any other startup logging fires, so the level applies repo-wide from the very first log line.
- Add a unit test for the parser (the env-var-to-slog-level translation), covering: each valid level (case mix), empty string, unrecognised value, missing env var.

The change should be additive: existing log call sites do not change.

**Acceptance:** `webhook/cmd/main.go` (or wherever startup logging is configured) reads and applies the env var; new test in the same package passes; `go test ./...` from the webhook module is green; existing tests unchanged.
