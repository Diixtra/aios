# pi Auth Transport Spike — Findings

**Date:** 2026-04-30
**Pi version:** 0.70.6
**Outcome:** <go | no-go | go-with-changes>

## A2 — PI_CODING_AGENT_DIR layout

After running `pi --mode json -p "ping"` once with no API key configured (so pi exits with an auth error before any session is persisted), `PI_CODING_AGENT_DIR=/pi-state` contains:

```
/pi-state/
├── auth.json     mode 0600   2 bytes  (initial content: `{}`)
└── sessions/     mode 0755   empty
```

- `auth.json` — JSON object holding provider auth state (refresh tokens, API keys, OAuth bundles). Mode 0600 means owner-read/write only — appropriate for a credential file. **The auth-broker store contract (Task B3) treats this file as opaque: the broker reads and atomically replaces the bytes; only pi parses the inner schema.**
- `sessions/` — per-session conversation logs land here once a real prompt completes. Empty when the run errors out before a session persists.
- **No `models.json`, `settings.json`, `cache/`, or other state files** in pi 0.70.6 with the `--no-extensions --no-skills --no-prompt-templates --no-context-files` flag set. These appear only when the relevant features are enabled (per the pi docs at `/usr/local/lib/node_modules/@mariozechner/pi-coding-agent/docs/`).
- File ownership inside the container: `root:root` by default (rootful pod). Production deployment (Task B14/B15) will run as a non-root UID with `fsGroup` set on the PVC; auth.json will be owned by that UID.
- Pi emits a `session` event with a UUID-tagged `id` and timestamp on every run, even when authentication fails. This event is the first JSONL line on stdout and is useful for correlating broker-side logs with pi-side runs.

**Atomic-write semantics not yet verified.** Determining whether pi writes auth.json in place vs `auth.json.tmp` + rename requires a real refresh event, which only happens after login. Deferred to **A6** (refresh-token rotation), where we can run `inotifywait` against the directory while a refresh fires. For Phase 0 design purposes the broker will assume **non-atomic writes** as a safe default and serialise broker-side bundle reads/writes with a flock — easy to relax later if A6 proves atomicity.

**Inotify probe (failed):** `apt-get install inotify-tools` silently failed in the container (rootful pod, no apt cache layered). Not blocking — the inotify check belongs in A6 anyway.

## A3 — Interactive login from headless container

**Outcome: (a) — pi's `/login` is interactive-only.** It cannot be triggered from `pi --mode json -p "/login"`; that invocation treats `/login` as a literal user message, fails the model call, and exits 1 because no API key is configured.

Confirmed by pi's bundled docs (`/usr/local/lib/node_modules/@mariozechner/pi-coding-agent/docs/providers.md`):

> Use `/login` in interactive mode, then select a provider

Also confirmed by `pi --help`: there is no `pi login` subcommand. Auth happens via:
1. **Interactive `/login` slash command** — runs an OAuth device flow with browser interaction; tokens land in `~/.pi/agent/auth.json`
2. **Pre-populated `auth.json`** at `$PI_CODING_AGENT_DIR/auth.json` — file format documented (per-provider object, `{ "type": "api_key", "key": "..." }` for keys; OAuth bundles use a different sub-shape we'll capture in A4)
3. **Environment variables** — `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, etc. (lower priority than auth.json)

**Consequence for the broker design:**

> "Auth-broker initiates a device-flow against `auth.openai.com` and posts the URL to Slack" — **as written in the spec, this is not achievable in pi 0.70.6 for ChatGPT subscription auth.** Pi's OAuth flow is entirely TUI-driven; the broker has no programmatic hook.

Two options for the design:
- **Option A (laptop bootstrap + laptop reauth).** User runs `/login` interactively on a TTY-backed host (their laptop) once; uploads the resulting `auth.json` to the broker via a one-shot `/v1/auth/bundle` POST. Pi's auto-refresh keeps the chain alive while the broker periodically invokes pi. When the refresh chain expires (long downtime, password change, revocation), Slack DM tells the user to re-run `/login` on their laptop and re-upload. **Phone-only reauth: not supported.** Phone gets the notification only.
- **Option B (web-TTY over Tailscale).** Broker hosts an interactive pi session over `ttyd` / xterm.js on a Tailscale-reachable URL. User taps the URL on phone, types `/login`, completes OAuth in mobile browser, exits. Broker now has the bundle. **Phone-only reauth: yes, but adds a non-trivial moving part (TTY-over-WebSocket service with auth, session lifecycle, etc.).**

**Recommendation:** Option A for v1. Adds a "bootstrap on laptop" one-time step but eliminates the web-TTY surface. Defer Option B to a follow-up if phone-only reauth proves load-bearing.

Captured stdout sample at `auth-broker-spike/samples/headless-login-events.jsonl` for posterity (just the `session` event + the auth-error message — no useful login event shape to capture from this path).

## A4 — Auth bundle portability
## A5 — Concurrent processes
## A6 — Refresh-token rotation
## A7 — Pi outbound HTTPS transport
## Decision
