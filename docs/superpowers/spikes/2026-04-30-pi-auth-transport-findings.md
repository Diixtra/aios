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
## A4 — Auth bundle portability
## A5 — Concurrent processes
## A6 — Refresh-token rotation
## A7 — Pi outbound HTTPS transport
## Decision
