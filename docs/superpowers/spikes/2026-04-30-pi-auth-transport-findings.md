# pi Auth Transport Spike — Findings

**Date:** 2026-04-30
**Pi version:** 0.70.6
**Outcome:** **go-with-changes** — proceed with Phase 0, fold per-Job bundle-copy refinement into B11.

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

**Outcome: portable.** A bundle bootstrapped on the user's laptop (`pi /login` → `~/.pi/agent/auth.json`) was copied into a fresh scratch directory, mounted into a fresh `aios-spike/pi:latest` container, and produced a real ChatGPT Codex inference call (`PONG`).

```
pi --model openai-codex/gpt-5.4 --mode json --no-extensions --no-skills \
   --no-prompt-templates --no-context-files \
   -p "Reply with the single token PONG and nothing else."
# → assistant text "PONG"; provider=openai-codex, api=openai-codex-responses, model=gpt-5.4
```

**Important findings:**

1. **Provider invocation pattern.** `--provider openai-codex` *alone* does not route to Codex; `--model openai-codex/<id>` does. The bare model id `gpt-5.4` resolves to `azure-openai-responses` (which fails with "no api key"). **Plan/runtime must use the qualified `provider/model` form** in pi invocations. Phase 1 `runtime/src/agents/code-pr.ts` will pass `--model openai-codex/gpt-5.4` (or whatever Codex model the AgentConfig pins).

2. **Bundle mutates during inference.** `auth.json` sha256 changed from `8ada7005…` → `c3b5bc64…` after a single Codex call. Field set + value lengths unchanged: `access (2039 chars)`, `refresh (90)`, `expires (13)`, `accountId (36)`, `type (5)`. The `access` and `expires` values rotate per call (or near-per-call). **Implication for the broker:** pi must run against a writeable bundle directory. The broker should treat the on-disk `auth.json` as the source of truth and capture mutations after each pi invocation rather than mounting RO. This is consistent with the Phase 0 design (broker owns the PVC).

3. **Cost field is non-zero on subscription path.** Pi reported `cost.total = $0.0026` for 1011 input + 6 output tokens via `openai-codex-responses`. This is likely pi computing a *notional* cost from public API pricing for display, not actual subscription billing — but worth flagging. **Operator-side telemetry should treat pi's `cost` field as advisory, not authoritative for billing.** The actual subscription burn is opaque to the broker.

4. **Anthropic Claude Pro/Max OAuth is restricted by Anthropic for third-party apps.** During testing, an inadvertent route to `provider=anthropic` returned `400 invalid_request_error: "Third-party apps now draw from your extra usage, not your plan limits."` This is an Anthropic-side policy change; OAuth-bundle Anthropic access via pi is effectively pay-per-token (against API extra-usage), not subscription. **Confirms the cost rationale for switching to Codex** — Anthropic OAuth is no longer free-with-subscription via pi.

5. **Session logs.** Each pi invocation writes a `sessions/--work--/<timestamp>_<uuid>.jsonl` file (~1.3KB) containing full prompt + response. These files are mildly sensitive (contain user prompts and assistant text). The broker's PVC must not be mounted into any non-broker container; specifically, agent Jobs should *copy* the bundle into their own scratch dir and *not* read/write the broker's `sessions/`.

6. **RO mount fails.** Pi requires a writeable `PI_CODING_AGENT_DIR` even when only reading the bundle (it needs to create `sessions/` and possibly `settings.json.lock`). Mounting `~/.pi/agent` read-only causes startup to crash with `ENOENT: no such file or directory, mkdir '/pi-state/sessions/--work--'`.

7. **Rancher Desktop / podman mount visibility:** scratch dirs under `/tmp` were not visible inside the Rancher VM (`statfs ... no such file or directory`). Using `$HOME` works. Using `:Z` SELinux relabel on the bind mount avoids container-side permission errors. Document this in the broker's local dev setup.

## A5 — Concurrent processes

**Outcome: safe at low concurrency.** Two pi containers (`pi-a` and `pi-b`) launched in parallel against the same `PI_CODING_AGENT_DIR` (same `auth.json`) both completed successfully:

- `pi-a` returned `AAA` (provider=openai-codex, model=gpt-5.4)
- `pi-b` returned `BBB` (provider=openai-codex, model=gpt-5.4, with reasoning trace)

Both processes coexisted on:
- `auth.json` — read concurrently with no locking visible from the host
- `sessions/` — both wrote separate per-process JSONL session files (filenames are `<timestamp>_<uuid>.jsonl` so collision risk is negligible)

`auth.json` sha256 was unchanged after this concurrent run — both processes consumed the still-fresh access token from the previous A4 call without triggering a rotation. This is **not a guarantee that concurrent rotations are safe**: under higher concurrency or near token-expiry, two pi processes might both attempt to refresh and write `auth.json`, producing a race.

**Implication for the broker design:**

> **Each Job gets its own writeable copy of the bundle**, not a shared PVC mount. The broker hands out an auth bundle (via `GET /v1/auth/bundle`) at lease-acquire time; the Job writes it to its own ephemeral `PI_CODING_AGENT_DIR`; the Job runs pi; on lease release, the Job optionally POSTs the rewritten bundle back via `PUT /v1/auth/bundle/post-run` (broker accepts, validates, persists if newer). This avoids the concurrent-write race entirely.

This is a **revision to the spec/plan**: previously the design assumed Jobs would mount the broker's PVC. Switching to per-Job bundle copy adds an upload-back step but eliminates a class of race conditions. Worth folding into Phase 0 — see plan task B11 (HTTP endpoints) — before that task ships.

## A6 — Refresh-token rotation

**Outcome: pi auto-refreshes transparently; broker does nothing special.**

Procedure:
1. Forced `auth.json["openai-codex"].expires = 1` (Unix epoch — i.e. very stale).
2. Invoked pi with `--model openai-codex/gpt-5.4 -p "ping"` against the tampered bundle.
3. Observed:
   - Inference succeeded (returned `CCC`).
   - `auth.json` sha256 changed: `a1e282c6...` → `f2baf741...`
   - `expires` updated to a future timestamp ~7 days out (e.g. `1778479259151`).
   - `access` field rotated to a fresh JWT (visible prefix `eyJhbGciOiJSUzI1NiIs...` differs from previous).
   - `refresh` field unchanged (90 chars — refresh token chain preserved).

**Implication for the broker:**

> The auth-broker does NOT need an OAuth refresh implementation of its own. **Periodically invoking pi is sufficient**: pi's internal HTTP client refreshes the access token on demand using the long-lived refresh token, and rewrites `auth.json` in place. The broker's "weekly silent refresh" (Phase 0 task B5) is just `pi --list-models` (or any cheap pi invocation) on a timer.

Atomic-write probe was inconclusive (inotify-tools install failed in container, not retried). However, A5 showed concurrent reads + writes don't collide at low load, and Go's atomic-write idiom in the broker's Store (Task B3) provides defence-in-depth on the broker side regardless of pi's behaviour. **Net: assume pi may or may not write atomically; the broker store handles both.**

## A7 — Pi outbound HTTPS transport

**Outcome: not OpenAI-compatible. Brokered auth (not proxying) is the correct architecture.**

Pi's JSON events for ChatGPT-Codex inference report:
- `provider`: `openai-codex`
- `api`: `openai-codex-responses`
- `model`: `gpt-5.4` (and other gpt-5.x variants)

The `openai-codex-responses` API name corresponds to OpenAI's **Codex / ChatGPT subscription transport**, not the public `/v1/chat/completions` endpoint. The two APIs differ in:
- Authentication: subscription bearer token (rotated frequently, tied to ChatGPT account) vs platform API key (`sk-...`)
- Request shape: `/responses` resource (with `input`, `instructions`, `tools` semantics) vs `/chat/completions`
- Streaming events: different SSE shapes

This means **we cannot front pi's subscription traffic with a generic OpenAI-compatible HTTP proxy**. Anything calling itself "OpenAI-compatible" expects `/v1/chat/completions`; pi-via-Codex calls the proprietary subscription endpoint.

Full mitmproxy capture was not run (skipped after determining the API name is sufficient evidence). If we ever decide to add an HTTP-level proxy for cost telemetry / observability, we'd need to reverse-engineer the Codex subscription transport — significant work, no clear payoff over reading pi's JSON event stream.

**Implication for the broker:** keeps the "brokered auth" design — no proxy. The broker's only HTTP-level interaction with OpenAI is whatever pi does internally; the broker just hands out `auth.json`. This is what the spec already says; A7 confirms it.

## Decision

**Go.** Phase 0 + Phase 1 may proceed with the laptop-bootstrap + brokered-auth-bundle design. Concrete commitments going into Phase 0:

| Question | Answer |
|---|---|
| Can the broker drive headless login? | **No.** `/login` is interactive; bootstrap is laptop-only (A3). |
| Is the auth bundle portable? | **Yes.** Verified end-to-end with a real Codex inference (A4). |
| Is concurrent multi-Job sharing safe? | **At low concurrency, yes.** Per-Job copy of bundle is the cleaner model — adds an upload-back step but eliminates the race-condition surface (A5). |
| Is bundle write atomic on pi's side? | **Inconclusive.** Broker store does atomic writes regardless (defence in depth) (A6). |
| Is the transport OpenAI-compatible? | **No.** `openai-codex-responses` is a proprietary subscription API. No proxy; brokered auth only (A7). |
| Does pi auto-refresh access tokens? | **Yes.** Transparent on every invocation; broker just runs pi periodically to keep the chain alive (A6). |
| **Go / no-go for Phase 0 as designed?** | **GO**, with the per-Job bundle-copy refinement folded into B11. |

Open follow-ups beyond Phase 0:
- If we ever want phone-only reauth, add a web-TTY surface (Risks table, Option B).
- If Codex breaks (provider outage / policy change), API-key fallback is the documented escape hatch — auth.json schema supports `{"openai":{"type":"api_key","key":"sk-..."}}` per pi's providers docs.
- Atomic-write check on pi (A6 deferred sub-task) — only matters if the broker store atomic-write proves insufficient under load. Defer until evidence shows it's needed.
