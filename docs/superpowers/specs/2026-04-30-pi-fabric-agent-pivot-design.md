# AIOS Pivot: Pi + Fabric Agent Architecture

**Date:** 2026-04-30
**Status:** Design — pending implementation plan
**Author:** james (with Claude)

## Summary

Replace the Claude Agent SDK path in the AIOS runtime with [pi](https://pi.dev) (Mario's terminal coding agent harness, MIT) running as an ephemeral per-task process. Prefer the user's existing ChatGPT subscription via pi's built-in OAuth provider, but validate that this is automatable before committing the migration; fall back to an API-key/OpenAI-compatible provider if the subscription transport cannot be safely shared with headless Jobs. Restore the operator's original intent of spawning multiple specialised agent types (code-pr, research, content, review, triage, ops). Promote fabric patterns into real pi skills/system-prompt resources. Fold TickTick AI features (official MCP where sufficient, otherwise a thin wrapper) into the system to eliminate `ticktick-sync` and related custom code. Keep `voice/` for interactive Slack push-to-talk.

## Goals

1. Lower per-token cost by routing inference through a ChatGPT subscription instead of metered Anthropic API.
2. Restore multiple agent types — each with its own system prompt, tooling, sandbox, and output destination.
3. Reduce custom code by leaning on pi's agent loop and TickTick's native AI features.
4. Preserve existing operator-spawns-Jobs orchestration model and per-Job sandboxing.
5. Notify the user on phone when auth needs attention; the reauth *action* runs on a TTY-backed laptop (Phase -1 A3 ruled out phone-only reauth in pi 0.70.6).

## Non-goals

- Horizontal scale beyond one ChatGPT subscription's concurrency cap. Operator becomes a priority queue, not a horizontal scaler.
- Replacing the K8s operator pattern. The operator stays; only the runtime changes.
- Migrating off Anthropic-based external integrations (Claude API used elsewhere is not in scope).
- Removing the voice gateway. Push-to-talk over Slack stays; the LocalAI infra is already in place.

## Constraints

1. **One subscription, one rate-limit bucket.** All agents share one ChatGPT account's concurrency. Default cap: 4 concurrent pi Jobs, configurable.
2. **Headless OAuth.** K8s Jobs cannot perform interactive login. Auth state must be owned by a long-running pod. Phase -1 A3 ruled out programmatic device-flow init in pi 0.70.6: bootstrap is laptop-only, broker holds and refreshes the bundle.
3. **Bundle is writeable, not transferable as-RO.** Pi 0.70.6 mutates `auth.json` per inference call (access-token rotation; A4 confirmed). The broker's PVC and any per-Job copy of the bundle MUST be RW. Read-only mounts crash pi at startup (sessions/ mkdir).
4. **Pi provider routing requires qualified model ids.** A4 confirmed `--model openai-codex/<id>` is mandatory; bare ids (e.g. `gpt-5.4`) misroute to other providers and fail. AgentConfig must store and pass models in the qualified form; operator validates at Job-build time.
5. **Pi provider reality.** Pi supports ChatGPT subscription login, custom providers, print mode, JSON mode, RPC mode, extensions, and skills. It does **not** ship MCP, sub-agents, permission prompts, or a daemon agent pool; MCP and policy enforcement must be extensions or external process/container controls.
6. **Anthropic OAuth is no longer subscription-effective for third-party apps.** A4 confirmed Anthropic now serves Claude Pro/Max OAuth tokens with `"third-party apps draw from your extra usage, not your plan limits"` — i.e. metered API extra-usage, not subscription. Strengthens the case for Codex; rules out Anthropic-OAuth as a fallback. Anthropic API key remains a paid fallback if Codex breaks.
7. **TickTick MCP requires premium.** Confirmed available to the user.
8. **Diixtra coding guidelines apply.** 80% test coverage floor per service. Test-first for business logic and security-sensitive paths. No mocking services we own.
9. **Refresh tokens expire.** Without active use, OpenAI refresh tokens can lapse. Auth-broker must refresh proactively and surface reauth before expiry.

## Approach

**Pi-as-CLI per Job (chosen).** Operator continues to spawn ephemeral K8s Jobs per `AgentTask`. Each Job container has pi installed and runs a thin agent entrypoint that performs preflight, launches pi, and performs postflight delivery. Pi runs in `--mode json` for machine-readable event streams, with explicit resource loading (`--no-extensions --no-skills --no-prompt-templates --no-context-files` plus per-agent `-e`, `--skill`, and `--system-prompt` inputs) so the Job is reproducible and does not accidentally inherit developer-local pi state.

**Auth is brokered, not assumed to be OpenAI-compatible.** The first implementation should use pi's native provider auth format where possible: a long-running `auth-broker` owns `PI_CODING_AGENT_DIR`/`auth.json`, refreshes credentials, and publishes a short-lived read-only auth bundle for Jobs. If Phase -1 proves that the selected pi subscription provider can be safely fronted as an OpenAI-compatible HTTP proxy, `auth-broker` may also expose that proxy; until then, do not design Jobs around `OPENAI_BASE_URL` alone.

**Concurrency is leased per Job.** Because subscription requests may go directly from pi to the provider instead of through a proxy, the global cap is enforced by the operator and `auth-broker` lease API (`/leases/acquire`, `/leases/release`). Holding a lease for the whole pi invocation is simpler and safer than trying to count individual provider calls. If later evidence shows this wastes too much capacity, replace it with a pi extension that leases around agent turns.

Alternatives rejected:

- **Long-running agent pool with task queue** — sidesteps per-Job auth complexity, but loses per-task isolation and is a bigger pivot from the existing operator design.
- **Pi SDK embedded in the current TS runtime** — good for future tighter integration, but the initial migration should avoid re-creating the existing 6-stage TS pipeline. Use the SDK later only for targeted wrappers/tests where CLI JSON mode is insufficient.
- **OpenAI-compatible auth proxy as the default assumption** — attractive, but only valid if pi's subscription provider can use a stable public-compatible transport and this is acceptable operationally.

## Architecture

```
                 ┌──────────────────────────────────────┐
GitHub ─────────►│ Webhook (Go)                         │
TickTick ───────►│   ingests GitHub, TickTick, Paperless│
Paperless ──────►│   creates AgentTask CR               │
Slack /commands ►│                                      │
Cron (triage) ─►│                                      │
                 │                                      │
                 │ Operator (Go) ──► spawns K8s Job     │ per task
                 └──────────────────────────────────────┘
                                                   │
                                       ┌───────────┴───────────┐
                                       │ Job: pi + SYSTEM.md   │
                                       │  + skills + extensions│
                                       │  + ToolPolicy         │
                                       └─────────┬─────────────┘
                                                 │ lease + auth bundle
                                       ┌─────────▼─────────────┐
                                       │ Auth Broker (new, Go) │
                                       │  pi OAuth holder      │
                                       │  Token refresh loop   │
                                       │  Job lease limiter    │
                                       │  Slack reauth bot     │
                                       │  optional HTTP proxy* │
                                       └─────────┬─────────────┘
                                                 │ HTTPS/provider transport
                                                 ▼
                                      pi-supported model provider

*Only if Phase -1 proves proxying is valid for the selected provider.
```

Side channels (Job → external):

- MCP servers: aios-search, memory, kubernetes, grafana, cloudflare, stripe, **+ exa, arxiv, obsidian-rest, ticktick (new)**
- Exa API for research-agent web search
- Obsidian Local REST API for vault read/write
- GitHub API for code-pr / review / triage / ops
- Slack API for thread updates

## Components

### Auth Broker (new — `auth-proxy/` or renamed `auth-broker/`)

**Language:** Go for the service; a small Node/pi helper container may be used for provider-specific login/refresh if pi's OAuth implementation is not exposed as a stable library from Go.

**Responsibilities:**

- Own the pi provider credentials (`auth.json`) on a writeable PVC (Phase -1 A4 ruled out RO mounts).
- Publish writeable per-Job auth bundle copies via `GET /v1/auth/bundle`. Jobs seed their own ephemeral `PI_CODING_AGENT_DIR`, run pi (which may rotate the access token in place), then upload the mutated bundle back via `POST /v1/auth/bundle/post-run`. The broker keeps whichever bundle has the newer `expires` (Spike A5 conclusion — eliminates concurrent-write races on the broker's stored bundle).
- Run pi periodically (default weekly) to keep the OAuth refresh-token chain alive (A6 confirmed pi auto-refreshes transparently — broker does no OAuth itself).
- Enforce a configurable **Job lease** cap (default 4) reflecting subscription concurrency limits.
- Surface metrics: `auth_broker_token_age_seconds`, `auth_broker_leases_active`, `auth_broker_lease_wait_seconds`, `auth_broker_refresh_total`, `auth_broker_post_run_uploads_total{accepted}`, `auth_broker_state` (Healthy / Warning / Expired / Awaiting).
- Drive the laptop-bootstrap reauth flow (see below).

**Auth lifecycle state machine (revised after Phase -1 A3):**

Pi 0.70.6's `/login` is interactive-only — there is no programmatic device-flow hook the broker can drive. The broker therefore *holds* and *refreshes* an auth bundle bootstrapped on a TTY-backed laptop; it does not *initiate* logins.

| State | Trigger | Action |
|---|---|---|
| Uninitialised | No bundle uploaded yet | Slack DM: bootstrap recipe (`pi /login` on laptop → `curl -F` upload to broker) |
| Healthy | Periodic pi invocation succeeds; bundle younger than warn threshold | None |
| Warning | Bundle older than warn threshold (default 23 days) | Slack DM: "AIOS auth approaching expiry — re-run `pi /login` on laptop and re-upload at your convenience" |
| Expired | Periodic pi invocation fails with auth error | Slack DM (urgent) + operator pauses queue: "AIOS paused — re-run `pi /login` on laptop and re-upload to resume" |
| Validating | Bundle just uploaded; broker running validation pi call | Slack thread update; operator queue stays paused |
| Healthy | Validation succeeds | Slack confirmation; operator drains queue |

**Bootstrap / reauth flow (laptop only):**

1. User runs `pi /login` on their laptop, completes OAuth in browser, exits pi.
2. User uploads the resulting auth bundle to the broker — one-shot authenticated POST: `curl -F bundle=@~/.pi/agent/auth.json -H "Authorization: Bearer $AIOS_BOOTSTRAP_TOKEN" https://auth-broker.aios.<cluster>/v1/auth/bundle`.
3. Broker stores the bundle on its PVC, runs a validation pi invocation (e.g. `pi --list-models` or a tiny `--mode json -p` ping), and transitions to Healthy.
4. Steady state: broker invokes pi periodically (the existing weekly-refresh scheduler) — pi auto-refreshes tokens transparently in `auth.json` per the providers.md docs.
5. When refresh fails or bundle is past the warn threshold, broker DMs the user with the recipe; user repeats steps 1-3 from any laptop with the project's `pi` installed.

**A `just bootstrap-auth` recipe** in the repo root or `auth-broker/` documents the exact upload command so the user does not have to remember the curl invocation.

**Phone reauth note:** out of scope for v1. If we later want phone-only reauth we add a web-TTY surface (e.g. `ttyd` over Tailscale) — see Risks. The Slack DM still goes to the phone; the action runs on laptop.

**Emergency trigger:** Slack slash command `/aios-reauth` forces a re-validation cycle (re-run pi against the current bundle and update state) and re-emits the bootstrap recipe DM. Useful when the user wants a clean state check or the previous DM got lost.

### Operator (`operator/`) — modified

**CRD changes:**

- `AgentTaskSpec.type` enum extended: `code-pr | research | content | review | triage | ops` (today: `coding | research`).
- New `AgentConfig` ConfigMap (or CRD if dynamic config is needed) per type. Defines:
  - SYSTEM.md path (relative to fabric-patterns volume)
  - ToolPolicy ref
  - Container image (default: shared `runtime` image)
  - **Pi model in qualified `provider/id` form** (e.g. `openai-codex/gpt-5.4`). Phase -1 A4 confirmed bare ids route to the wrong provider; the operator validates this at Job-build time and rejects bare ids early.
  - MCP servers to mount
  - Env vars (incl. agent-specific Exa API key, Obsidian REST URL, etc.)
  - Output destination (gh PR / vault path / issue comment / Slack thread)

**Status changes:**

- `phase`: `Pending | Running | Completed | Failed | Blocked`
- `agentType`: copied from spec for query convenience
- `outputRef`: PR URL / vault path / issue # / etc.
- `tokensUsed` / `costUSD`: extracted from pi JSON events. **Treat as advisory, not authoritative**: Phase -1 A4 confirmed pi reports notional API-equivalent cost for ChatGPT-Codex subscription calls (the user is billed against subscription quota, not the displayed dollar amount). Useful for *relative* trend monitoring (which agent type burns more), not for actual subscription accounting.
- `leaseId`: auth-broker lease held by the Job, for debugging/recovery
- `reauthBlocked` bool: set when auth-broker reports provider auth failure globally

**Behaviour changes:**

- Job template mounts: fabric-patterns volume, ToolPolicy, AgentConfig, per-job `PI_CODING_AGENT_DIR`, and auth-broker Service env. Pi is installed into the shared `runtime` container image at build time (not mounted).
- Before creating a Job, acquire an auth-broker lease for the agent type/priority; release on terminal Job status or lease TTL expiry.
- Priority queue: when lease wait time or queue depth exceeds threshold for >60s, operator pauses lower-priority Jobs.
- On global provider auth failure reported by auth-broker or any Job: operator marks new tasks `Blocked: ReauthRequired`; in-flight Jobs run to completion if possible; resumes once auth-broker returns Healthy.

**Priority order (default, configurable):**

1. ops (alert-driven, time-sensitive)
2. review (blocks merges)
3. code-pr (user is waiting)
4. research
5. content
6. triage (scheduled, async)

### Runtime (`runtime/`) — slimmed

**Removed:**

- `runtime/src/pipeline/` (research, understand, plan, implement, verify, deliver). Pi owns the agent loop.
- `@anthropic-ai/sdk` dependency.
- `runtime/src/fabric.ts` (replaced by pi extension).
- `runtime/src/sandbox.ts` (replaced by pi extension).
- `runtime/src/agent-tools.ts` (replaced by pi MCP / tool config).

**Added — per-agent entrypoints (`runtime/src/agents/`):**

- `code-pr.ts` — preflight: clone repo, fetch issue, build context bundle. Invoke pi. Postflight: open PR via `gh`, post Slack thread.
- `research.ts` — preflight: extract topic. Invoke pi. Postflight: write brief to vault via Obsidian REST, post Slack.
- `content.ts` — preflight: fetch TickTick item via TickTick MCP. Invoke pi. Postflight: write to vault, post Slack.
- `review.ts` — preflight: `gh pr view` + diff. Invoke pi. Postflight: post review comments via `gh pr review`, Slack summary.
- `triage.ts` — preflight: `gh issue list`. Invoke pi. Postflight: Slack digest of actions taken.
- `ops.ts` — preflight: fetch alert context, read-only `kubectl describe`. Invoke pi. Postflight: open manifest-fix PR, Slack diagnosis.

Each entrypoint is a thin shell. Pi handles the agent loop, built-in/custom tool dispatch, and context compaction.

**Job invocation contract (sketch):**

```bash
export PI_CODING_AGENT_DIR=/var/run/aios/pi-agent   # brokered, per-job, read-only except sessions/tmp
pi \
  --mode json \
  --no-session \
  --no-context-files \
  --no-extensions -e /app/pi-extensions/tool-policy/index.ts \
  -e /app/pi-extensions/slack-thread/index.ts \
  -e /app/pi-extensions/mcp-bridge/index.ts \
  -e /app/pi-extensions/result-reporter/index.ts \
  --no-skills --skill /fabric/extract-requirements/SKILL.md \
  --system-prompt "$(cat /agent-config/SYSTEM.md)" \
  --model "$AIOS_PI_MODEL" \
  "$(cat /work/prompt.md)"
```

The wrapper captures JSONL events, preserves the full log as a Job artifact, and reads the result from the `deliver_result` tool event rather than scraping final assistant text.

### Pi extensions (`runtime/pi-extensions/` — new)

Pi extensions are TypeScript modules loaded explicitly with `-e` per Job. They can register tools, block/modify tool calls, observe provider responses, and stream events; they are not a security boundary by themselves.

- **Tool-policy** — reads ToolPolicy, calls `pi.setActiveTools()`, and blocks risky `bash`/`write`/`edit` calls in `tool_call`. Pair with container hardening and Kubernetes `NetworkPolicy` for real isolation.
- **Slack-thread** — listens to `message_update`, `tool_execution_*`, and terminal events; posts compact live updates into the task's Slack thread.
- **MCP-bridge** — registers pi tools that call approved MCP servers (aios-search, memory, kubernetes, grafana, cloudflare, stripe, exa, arxiv, obsidian-rest, ticktick). Pi has no built-in MCP support.
- **Result-reporter** — registers a `deliver_result` tool. Agents must call it with structured JSON (`status`, `summary`, `artifacts`, `comments`, `nextActions`) before finishing, avoiding brittle parsing of final prose.
- **Fabric-loader** — optional helper that discovers project-local fabric skills and adds them through pi's resource discovery. The preferred path is still explicit `--skill` flags from AgentConfig for reproducibility.

### Fabric patterns — promoted to first-class pi resources

- Each pattern becomes a real Agent Skill directory with `SKILL.md` frontmatter (`name`, `description`) that satisfies pi/Agent Skills discovery rules. Do **not** invent `pi-skill.yaml` unless an extension consumes it; pi will not load it natively.
- Keep long reference material beside the skill and link it from `SKILL.md` for progressive disclosure.
- Per-agent system prompt is composed into a generated file or passed via `--system-prompt`: `agent-base.md` + agent-type-specific instructions. Example: research-agent system prompt = `research-base.md` + safety/output requirements; skills loaded explicitly = `extract-requirements`, `source-evaluation`, etc.
- AgentConfig lists exact skills to load (`--skill fabric-patterns/extract-requirements/SKILL.md`) and whether they are merely available or required in the opening prompt.

### New MCP servers (`mcp-servers/`)

- **`exa/`** — web search via Exa API.
- **`arxiv/`** — paper search, abstract extraction, full-text fetch.
- **`obsidian-rest/`** — read/write notes via Obsidian Local REST API plugin.
- **`ticktick/`** — uses TickTick's official MCP (premium feature). If the official MCP surface proves insufficient for our needs, fallback is a thin custom wrapper around TickTick's REST API; scope to be assessed in a 1-day spike at the start of Phase 4.

### Webhook (`webhook/`) — modified

- Adds TickTick webhook handler. Trigger rules (configurable):
  - Tag `#aios-research` → research-agent
  - Tag `#aios-meeting` → content-agent (meeting notes draft from transcript)
  - Tag `#aios-draft` → content-agent
  - Tag `#aios-triage` → triage-agent on demand
- Adds Slack slash command handler for `/aios-reauth` (calls auth-broker `/admin/revalidate`).
- Adds Grafana alert webhook for ops-agent.
- Existing GitHub + Paperless handlers preserved.

### Removed components

- **`ticktick-sync/`** — bidirectional sync logic deprecated. Webhook ingestion absorbed into `webhook/`. Read/write to TickTick happens via TickTick MCP from agent Jobs that need it.
- **`runtime/src/pipeline/`, `fabric.ts`, `sandbox.ts`, `agent-tools.ts`** — see Runtime above.
- **`@anthropic-ai/sdk`** dependency.

### Preserved components (unchanged)

- `voice/` — Slack push-to-talk frontend remains.
- `mcp-proxy/` — MCP sampling bridge remains.
- `aios-search/` — Obsidian vault embeddings remain.
- Existing MCP servers (kubernetes, grafana, cloudflare, stripe, dot-ai memory).
- Existing Paperless webhook integration.

## Per-agent data flow

### code-pr

```
GitHub issue → webhook → AgentTask{type=code-pr, inputRef=gh://issue/123}
operator → Job(image=runtime, entrypoint=agents/code-pr.ts)
  preflight: clone repo, fetch issue body, build context bundle (aios-search + memory MCP)
  pi: system prompt = code-pr-base; skills = extract-requirements + write-pull-request
       tools: read/write/edit/bash, gh wrapper, MCP{aios-search, memory}
       prompt: "Implement issue #123. Run tests. Commit on a feature branch."
  postflight: read pi's structured output (JSON), open PR via gh, post Slack thread link
```

### research

```
TickTick #aios-research OR Slack /research → AgentTask{type=research}
  preflight: extract topic, fetch any linked URLs
  pi: system prompt = research-base; skills = extract-requirements + source-evaluation
       tools: MCP{exa, arxiv, obsidian-rest, memory, aios-search}, file-io tmp only (no repo write)
       prompt: "Research <topic>. Cite sources. Produce a structured brief."
  postflight: write brief to Obsidian vault via REST at notes/research/YYYY-MM-DD-<slug>.md, post Slack
```

### content (incl. meeting transcripts from TickTick)

```
TickTick #aios-meeting OR #aios-draft → AgentTask{type=content, inputRef=ticktick://item/<id>}
  preflight: fetch TickTick item (transcript or topic) via TickTick MCP
  pi: system prompt = content-base; skills = improve-prompt + voice-style
       tools: MCP{ticktick, obsidian-rest, memory, aios-search}, file-io tmp
       prompt: "Draft <type> from this input. Match my voice. Output as markdown."
  postflight: write to Obsidian vault, post Slack with link
```

### review

```
GitHub PR opened/labelled → webhook → AgentTask{type=review, inputRef=gh://pr/456}
  preflight: gh pr view + diff
  pi: system prompt = review-base; skills = review-code + find-hidden-bugs
       tools: gh (read-only), MCP{aios-search, memory}
       prompt: "Review PR #456. Flag bugs, security risks, scope creep."
  postflight: post review comments via gh pr review --comment, Slack summary
```

### triage (scheduled)

```
CronJob creates AgentTask{type=triage} (default: weekly Monday 09:00)
  preflight: gh issue list across configured repos
  pi: system prompt = triage-base; skills = extract-requirements
       tools: gh (issue read/edit/comment), MCP{memory, ticktick}
       prompt: "Sweep issues. Label, prioritise, close stale (>90d, no activity)."
  postflight: Slack digest of actions taken
```

### ops

```
Grafana alert → webhook → AgentTask{type=ops, inputRef=alert://<id>}
  preflight: fetch alert context, kubectl describe relevant resources (read-only)
  pi: system prompt = ops-base; skills = find-hidden-bugs + incident-diagnosis
       tools: MCP{kubernetes (read-only), grafana, cloudflare}, gh, git, shell (read-only kubectl allowlist)
       prompt: "Diagnose. Propose manifest fix as PR. Do not apply directly."
  postflight: open PR with manifest change, Slack with diagnosis + PR link
```

## Error handling

| Failure | Detection | Behaviour |
|---|---|---|
| Pi exits non-zero | Job container exit code | Operator marks AgentTask `Failed`; Slack alert with last 50 log lines |
| Tool denied by ToolPolicy | Pi extension emits structured event | Job continues; pi sees the denial in its loop and adapts |
| MCP server unreachable | Pi MCP extension surfaces error | Pi retries 3x with backoff; if still failing, agent reports degraded result |
| Provider auth failure | pi JSON error, auth-broker validation failure, or provider 401-equivalent | Operator marks task `Blocked: ReauthRequired`; **does not retry**; pauses queue; auth-broker triggers reauth flow |
| Lease unavailable / provider rate limit | auth-broker lease wait, provider 429-equivalent, or pi error | Operator keeps task Pending/Blocked with jitter; pauses lower-priority tasks |
| Provider outage | pi/provider 5xx-equivalent | Job retries up to 3x with exponential backoff if idempotent; otherwise fails with Slack alert |
| Test verification fails (code-pr only) | `deliver_result` status or postflight test command | Entrypoint sends up to 3 follow-up prompts to pi with the failure log; if still failing, opens PR as **draft** with failure log attached |
| Vault write conflict | Obsidian REST 409 | Append timestamp suffix and retry once |

**Concurrency limiter:** Auth-broker lease table (default 4 active Jobs). Operator uses lease wait depth/age to pause lower-priority Jobs when pressure exceeds threshold for >60s. Leases have TTL and are renewed by the Job wrapper so crashed Jobs do not permanently consume capacity.

**Reauth alarms (Slack DM only, never a channel):**

- 7 days before expiry: gentle nudge, system continues.
- 1 day before expiry: louder ping, operator marks new non-priority tasks `Blocked: ReauthSoon`.
- On expiry: incident-style ping, repeating hourly until resolved; queue paused.

## Testing strategy

**Unit (Diixtra default tooling per language):**

- Auth-broker: token-refresh/login state machine (proptest-style — random clock advances, random provider auth/rate-limit/outage mixes); lease limiter; Slack notifier.
- Operator: AgentTask reconciler (envtest, real CRDs, fake k8s API); priority queue logic.
- Webhook: signature validation; TickTick event → AgentTask mapping; `/aios-reauth` slash command.
- Pi extensions (tool-policy, slack-thread, result-reporter, MCP-bridge, fabric-loader): TS unit tests for pure logic plus SDK-backed tests using `createAgentSession()`/in-memory sessions where possible.

**Integration:**

- Per-agent-type smoke test: run a real Job container with pi in `--mode json` against a mock provider or canned pi session, assert `deliver_result` and destination side effects.
- Auth-broker: real broker service with mock provider login/refresh endpoint, verify token persistence across pod restart (Secret/PVC + restart test), lease expiry, and Slack reauth callbacks.
- Pi provider compatibility spike: run a disposable real login in a non-prod namespace, verify headless Job can use the brokered auth bundle and collect usage/errors without developer-local state.
- TickTick MCP: integration test against a real TickTick sandbox account (premium-tier feature flag).

**End-to-end (Chainsaw, on a kind cluster):**

- Drop a fixture GitHub-issue webhook → verify PR opens within 10 min (with mock model-provider responses).
- Drop a fixture TickTick meeting webhook → verify Obsidian vault writes the note.
- Trigger a fake Grafana alert → verify ops PR opens.

**Coverage floor:** 80% per service (Diixtra default), enforced in CI from day one for new services (auth-broker, MCP servers). Existing services retain their current coverage during migration but cannot regress.

**No mocking of services we own.** Auth-broker integration tests use a real auth-broker + mock provider; operator tests use real operator + envtest. Mocks only for model providers, GitHub, TickTick, Exa, Slack APIs.

## Migration plan

Nothing breaks during transition. Old and new run side-by-side gated by `AgentConfig.engine: claude-sdk | pi`.

**Phase -1 — pi/provider/auth feasibility spike (1-2 days, required before buildout)**

- Pin a pi version and record the exact provider/model target (`openai`/Codex subscription vs API-key-compatible fallback).
- In a disposable namespace, run pi non-interactively with an isolated `PI_CODING_AGENT_DIR`, `--mode json`, explicit `--no-*` resource flags, and a minimal prompt.
- Prove that a Job can consume brokered credentials without interactive login and without mounting developer-local state.
- Verify usage/error events are observable enough for operator status and Slack reporting.
- Decide: native pi auth bundle (preferred), custom provider extension, or API-key fallback. Update this spec with the chosen transport before Phase 0.

**Phase 0 — auth-broker lands with laptop-bootstrap auth (2-4 days)**

- Build, deploy.
- Bootstrap once: run `pi /login` on laptop, `curl -F bundle=@auth.json` upload.
- Implement lease API and operator-side lease acquisition/release.
- Verify Slack DM notification fires when the bundle ages past the warn threshold (synthetic clock advance in test).
- Verify weekly refresh/validation works (broker invokes pi; pi auto-refreshes tokens; bundle rewrites observed on PVC).
- No production impact — service exists but is dormant.

**Phase 1 — code-pr agent rebuilt on pi (~1 week)**

- New `runtime/src/agents/code-pr.ts` entrypoint, consuming the auth-broker already deployed in Phase 0.
- Pi extensions: tool-policy, slack-thread, result-reporter, MCP-bridge, fabric-loader (initial set, sufficient for code-pr).
- Feature flag enables side-by-side: same issue can run through old SDK path and new pi path; compare PR quality.
- Cut over once equal-or-better; keep claude-sdk path for 2 weeks rollback.

**Phase 2 — research agent (new) (~3-4 days)**

- New entrypoint, exa MCP, arxiv MCP, obsidian-rest MCP.
- Wire Slack `/research` slash command and TickTick `#aios-research` trigger.

**Phase 3 — review + triage agents (~1 week each)**

- New entrypoints, gh-review tooling.
- CronJob for triage.

**Phase 4 — content agent + TickTick consolidation (~1 week)**

- Spike (1 day): validate TickTick official MCP surface is sufficient.
- TickTick MCP integration.
- Move TickTick webhook ingestion into `webhook/`.
- **Decommission `ticktick-sync/`** after webhook + MCP path is live for 2 weeks.

**Phase 5 — ops agent (~1 week)**

- Grafana alert webhook handler.
- kubernetes MCP read-only mode.

**Phase 6 — cleanup (~2 days)**

- Remove `runtime/src/pipeline/`, `fabric.ts`, `sandbox.ts`, `agent-tools.ts`.
- Remove `@anthropic-ai/sdk` dependency.
- Remove the `engine: claude-sdk` feature flag.
- Voice gateway preserved; not deprecated.

**Total estimate:** ~6 weeks focused work, ~10-12 weeks calendar at homelab pace.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| TickTick official MCP surface too narrow | Medium | Phase 4 grows by ~3 days | Phase 4 starts with 1-day spike; fallback is thin custom TickTick MCP |
| Subscription concurrency cap lower than expected | Medium | Throughput pain | Auth-broker lease cap is configurable; priority queue absorbs the limit |
| Subscription auth cannot be safely shared with headless Jobs | Medium | Cost premise or architecture changes | Phase -1 blocks buildout; fallback to API-key provider or custom provider extension |
| Refresh token revoked by provider policy change | Low | All agents paused until reauth | Slack DM with laptop-recipe; weekly validation keeps chain alive while user re-runs `pi /login` and re-uploads |
| Phone-only reauth desired | Medium (UX preference) | Reauth requires laptop access | Defer; if load-bearing later, add a `ttyd` over Tailscale surface (Option B from A3 findings) |
| Pi extension API changes (pre-1.0) | Medium | Local breakage on upgrade | Pin pi version; upgrade behind a feature flag; integration tests run against pinned pi |
| Cost premise wrong (subscription not actually cheaper) | Low | Pivot wasted effort | Phase 1 produces real comparison data before full commitment |
| Headless auth in K8s Job complexity underestimated | Medium | Phase 0 grows | Phase -1 proves the exact transport; no agents touch it until proven |

## Open questions deferred to implementation

- Exact TickTick MCP tool surface — resolved in Phase 4 spike.
- Chosen pi provider/auth transport — resolved in Phase -1 and documented before Phase 0.
- Pi version pin — latest stable at Phase -1 start; revisit before Phase 1.
- Whether `AgentConfig` should be a ConfigMap or a new CRD — start with ConfigMap; promote to CRD only if dynamic config or status is needed.
- Auth-broker on multi-replica vs single-pod — start single-pod (auth state is hard to share); promote to leader-elected if availability becomes an issue.
- Pi RPC mode adoption. v1 uses `--mode json` (one-shot, matches ephemeral Job model). RPC mode (`pi --mode rpc --no-session`, JSONL over stdin/stdout, with `prompt`/`steer`/`followUp`/`cancel` and streamed `message_update`/`tool_execution_*` events) unlocks mid-flight steering, multi-turn sessions, and clean operator-side interrupt — worth revisiting once the JSON-mode baseline ships and we have evidence of where its limitations bite (e.g. wasted budget on runaway turns, awkward multi-turn content workflows, priority preemption needs).

## Success criteria

1. All 6 agent types ship and run successfully on real tasks for at least 2 weeks each.
2. Monthly inference cost is measurably lower than the pre-pivot Anthropic API spend.
3. Manual reauth events are <1/month on average and resolved in under 2 minutes on a laptop (run `pi /login` + `just bootstrap-auth` upload).
4. `ticktick-sync/` is removed; voice gateway preserved.
5. Test coverage ≥80% on auth-broker, operator changes, pi extensions, and new MCP servers.
6. No regression in PR quality between SDK and pi-based code-pr agent (judged on PRs from a 2-week side-by-side comparison).
