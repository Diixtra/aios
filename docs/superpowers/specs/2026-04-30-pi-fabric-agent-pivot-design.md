# AIOS Pivot: Pi + Fabric Agent Architecture

**Date:** 2026-04-30
**Status:** Design — pending implementation plan
**Author:** james (with Claude)

## Summary

Replace the Claude Agent SDK in the AIOS runtime with [pi](https://pi.dev) (Mario's terminal coding agent harness, MIT) backed by the user's existing ChatGPT subscription via OAuth. Restore the operator's original intent of spawning multiple specialised agent types (code-pr, research, content, review, triage, ops). Fold TickTick AI features (official MCP, voice/transcription) into the system to eliminate `ticktick-sync` and related custom code. Keep `voice/` for interactive Slack push-to-talk.

## Goals

1. Lower per-token cost by routing inference through a ChatGPT subscription instead of metered Anthropic API.
2. Restore multiple agent types — each with its own system prompt, tooling, sandbox, and output destination.
3. Reduce custom code by leaning on pi's agent loop and TickTick's native AI features.
4. Preserve existing operator-spawns-Jobs orchestration model and per-Job sandboxing.
5. Make reauthentication a phone-driven operation with no kubectl required.

## Non-goals

- Horizontal scale beyond one ChatGPT subscription's concurrency cap. Operator becomes a priority queue, not a horizontal scaler.
- Replacing the K8s operator pattern. The operator stays; only the runtime changes.
- Migrating off Anthropic-based external integrations (Claude API used elsewhere is not in scope).
- Removing the voice gateway. Push-to-talk over Slack stays; the LocalAI infra is already in place.

## Constraints

1. **One subscription, one rate-limit bucket.** All agents share one ChatGPT account's concurrency. Default cap: 4 concurrent inferences, configurable.
2. **Headless OAuth.** K8s Jobs cannot perform interactive login. Auth state must be owned by a long-running pod (auth-proxy).
3. **TickTick MCP requires premium.** Confirmed available to the user.
4. **Diixtra coding guidelines apply.** 80% test coverage floor per service. Test-first for business logic and security-sensitive paths. No mocking services we own.
5. **Refresh tokens expire.** Without active use, OpenAI refresh tokens lapse in 30-90 days. Auth-proxy must refresh proactively.

## Approach

**Pi-as-CLI per Job (chosen).** Operator continues to spawn ephemeral K8s Jobs per AgentTask. Each Job container has pi installed, mounts a per-agent SYSTEM.md (composed from `agent-base.md` + relevant fabric patterns), a ToolPolicy ConfigMap, and the auth-proxy environment configuration. Pi runs in `print/JSON` mode and points `OPENAI_BASE_URL` at the auth-proxy in-cluster Service.

Alternatives rejected:

- **Long-running agent pool with task queue** — sidesteps per-Job auth complexity, but loses per-task isolation and is a bigger pivot from the existing operator design.
- **Pi RPC mode embedded in the current TS runtime** — smallest code delta but retains the 6-stage TS pipeline that pi is meant to obviate.

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
                                       │  + ToolPolicy + tools │
                                       └─────────┬─────────────┘
                                                 │ OpenAI-compatible HTTP
                                       ┌─────────▼─────────────┐
                                       │ Auth Proxy (new, Go)  │
                                       │  ChatGPT OAuth holder │
                                       │  Token refresh loop   │
                                       │  Concurrency limiter  │
                                       │  Slack reauth bot     │
                                       └─────────┬─────────────┘
                                                 │ HTTPS
                                                 ▼
                                            api.openai.com
```

Side channels (Job → external):

- MCP servers: aios-search, memory, kubernetes, grafana, cloudflare, stripe, **+ exa, arxiv, obsidian-rest, ticktick (new)**
- Exa API for research-agent web search
- Obsidian Local REST API for vault read/write
- GitHub API for code-pr / review / triage / ops
- Slack API for thread updates

## Components

### Auth Proxy (new — `auth-proxy/`)

**Language:** Go (consistent with operator/webhook).

**Responsibilities:**

- Hold ChatGPT OAuth refresh token in a 1Password Operator-backed Secret.
- Expose OpenAI-compatible `/v1/chat/completions` and `/v1/embeddings` endpoints on a ClusterIP Service.
- Run a token refresh loop with jitter; perform a no-op refresh weekly to keep the chain alive even when traffic is quiet.
- Enforce a configurable concurrency cap (default 4) reflecting subscription concurrency limits.
- Surface metrics: `auth_proxy_token_age_seconds`, `auth_proxy_requests_total`, `auth_proxy_concurrent_inflight`, `auth_proxy_rate_limit_remaining`, `auth_proxy_state` (Healthy / Warning / Expired / Awaiting).
- Drive the phone-based reauth flow (see below).

**Phone-based reauth state machine:**

| State | Trigger | Action |
|---|---|---|
| Healthy | Routine refreshes succeed | None |
| Warning | 7 days before expiry | Slack DM: "Approaching expiry, tap to reauthenticate now if convenient" |
| Expired | Refresh fails / 401 from OpenAI | Slack DM (urgent): "AIOS paused — tap to resume" + operator pauses queue |
| Awaiting | Device-flow polling in progress | Slack thread updates with progress |
| Healthy | Device-flow returns token | Slack confirmation, operator unpauses queue |

**Reauth flow (no kubectl):**

1. Auth-proxy initiates OAuth device-flow against `auth.openai.com`.
2. Receives `verification_uri_complete` (a public HTTPS URL containing the device code).
3. Posts to user's Slack DM (not a channel — reauth links are sensitive) with a single button.
4. User taps button on phone, lands on OpenAI auth page (already logged in), taps Allow.
5. Auth-proxy is polling token endpoint, captures token within ~3 seconds.
6. Posts confirmation; operator drains queue.

**Idempotency:** if a device-flow is already pending, repeated trigger requests return the same URL rather than initiating a second flow. Device codes have a short TTL (~15 min); after expiry, a fresh flow can be initiated.

**Emergency trigger:** Slack slash command `/aios-reauth` forces a fresh device-flow URL on demand. Implemented as a thin handler in `webhook/` that calls `auth-proxy`'s `/admin/start-reauth` endpoint.

### Operator (`operator/`) — modified

**CRD changes:**

- `AgentTaskSpec.type` enum extended: `code-pr | research | content | review | triage | ops` (today: `coding | research`).
- New `AgentConfig` ConfigMap (or CRD if dynamic config is needed) per type. Defines:
  - SYSTEM.md path (relative to fabric-patterns volume)
  - ToolPolicy ref
  - Container image (default: shared `runtime` image)
  - MCP servers to mount
  - Env vars (incl. agent-specific Exa API key, Obsidian REST URL, etc.)
  - Output destination (gh PR / vault path / issue comment / Slack thread)

**Status changes:**

- `phase`: `Pending | Running | Completed | Failed | Blocked`
- `agentType`: copied from spec for query convenience
- `outputRef`: PR URL / vault path / issue # / etc.
- `tokensUsed`: extracted from auth-proxy response headers (best-effort)
- `reauthBlocked` bool: set when auth-proxy reports 401 globally

**Behaviour changes:**

- Job template mounts: fabric-patterns volume, ToolPolicy, AgentConfig, auth-proxy Service env. Pi is installed into the shared `runtime` container image at build time (not mounted).
- Priority queue: when `auth_proxy_queue_depth` exceeds threshold for >60s, operator pauses lower-priority Jobs.
- On global 401 reported by any Job: operator marks new tasks `Blocked: ReauthRequired`; in-flight Jobs run to completion; resumes once auth-proxy returns Healthy.

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

Each entrypoint is a thin shell. Pi handles the agent loop, tool dispatch, and context compaction.

### Pi extensions (`runtime/pi-extensions/` — new)

- **Sandbox** — allowlist shell commands (per ToolPolicy), path protection, deny network egress except via approved MCP servers.
- **Slack-thread** — pipe assistant turns into a Slack thread for live observability.
- **MCP** — point pi at: aios-search, memory, kubernetes, grafana, cloudflare, stripe, exa, arxiv, obsidian-rest, ticktick.
- **Fabric-skill** — register each `fabric-patterns/*/system.md` as a pi Agent Skill, slash-invokable mid-conversation.

### Fabric patterns — promoted to first-class config

- Each pattern gets a `pi-skill.yaml` sibling defining when the skill is offered (auto-load for X agent type, slash-invokable, etc.).
- Per-agent SYSTEM.md is composed: `agent-base.md` + agent-type-specific patterns. Example: research-agent's SYSTEM.md = `research-base.md` + `extract_requirements/system.md`.

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
- Adds Slack slash command handler for `/aios-reauth` (calls auth-proxy `/admin/start-reauth`).
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
  pi: SYSTEM.md = code-pr-base + extract_requirements + write_pull_request
       tools: shell, file-io, gh, MCP{aios-search, memory}
       prompt: "Implement issue #123. Run tests. Commit on a feature branch."
  postflight: read pi's structured output (JSON), open PR via gh, post Slack thread link
```

### research

```
TickTick #aios-research OR Slack /research → AgentTask{type=research}
  preflight: extract topic, fetch any linked URLs
  pi: SYSTEM.md = research-base + extract_requirements
       tools: MCP{exa, arxiv, obsidian-rest, memory, aios-search}, file-io tmp only (no repo write)
       prompt: "Research <topic>. Cite sources. Produce a structured brief."
  postflight: write brief to Obsidian vault via REST at notes/research/YYYY-MM-DD-<slug>.md, post Slack
```

### content (incl. meeting transcripts from TickTick)

```
TickTick #aios-meeting OR #aios-draft → AgentTask{type=content, inputRef=ticktick://item/<id>}
  preflight: fetch TickTick item (transcript or topic) via TickTick MCP
  pi: SYSTEM.md = content-base + improve_prompt
       tools: MCP{ticktick, obsidian-rest, memory, aios-search}, file-io tmp
       prompt: "Draft <type> from this input. Match my voice. Output as markdown."
  postflight: write to Obsidian vault, post Slack with link
```

### review

```
GitHub PR opened/labelled → webhook → AgentTask{type=review, inputRef=gh://pr/456}
  preflight: gh pr view + diff
  pi: SYSTEM.md = review-base + review_code + find_hidden_bugs
       tools: gh (read-only), MCP{aios-search, memory}
       prompt: "Review PR #456. Flag bugs, security risks, scope creep."
  postflight: post review comments via gh pr review --comment, Slack summary
```

### triage (scheduled)

```
CronJob creates AgentTask{type=triage} (default: weekly Monday 09:00)
  preflight: gh issue list across configured repos
  pi: SYSTEM.md = triage-base + extract_requirements
       tools: gh (issue read/edit/comment), MCP{memory, ticktick}
       prompt: "Sweep issues. Label, prioritise, close stale (>90d, no activity)."
  postflight: Slack digest of actions taken
```

### ops

```
Grafana alert → webhook → AgentTask{type=ops, inputRef=alert://<id>}
  preflight: fetch alert context, kubectl describe relevant resources (read-only)
  pi: SYSTEM.md = ops-base + find_hidden_bugs
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
| Auth-proxy 401 (token revoked) | HTTP 401 | Operator marks task `Blocked: ReauthRequired`; **does not retry**; pauses queue; auth-proxy triggers reauth flow |
| Auth-proxy 429 (rate limit) | HTTP 429 | Job backs off with jitter; operator pauses lower-priority tasks |
| Auth-proxy 503 (downstream OpenAI down) | HTTP 503 | Job retries up to 3x with exponential backoff |
| Test verification fails (code-pr only) | Pi structured output | Pi self-retries up to 3 turns within the same Job; if still failing, opens PR as **draft** with failure log attached |
| Vault write conflict | Obsidian REST 409 | Append timestamp suffix and retry once |

**Concurrency limiter:** Auth-proxy in-process semaphore (default 4). Operator subscribes to `auth_proxy_queue_depth` and pauses lower-priority Jobs when depth exceeds threshold for >60s.

**Reauth alarms (Slack DM only, never a channel):**

- 7 days before expiry: gentle nudge, system continues.
- 1 day before expiry: louder ping, operator marks new non-priority tasks `Blocked: ReauthSoon`.
- On expiry: incident-style ping, repeating hourly until resolved; queue paused.

## Testing strategy

**Unit (Diixtra default tooling per language):**

- Auth-proxy: token-refresh state machine (proptest-style — random clock advances, random 401/429/503 mixes); concurrency limiter; Slack notifier.
- Operator: AgentTask reconciler (envtest, real CRDs, fake k8s API); priority queue logic.
- Webhook: signature validation; TickTick event → AgentTask mapping; `/aios-reauth` slash command.
- Pi extensions (sandbox, slack-thread, fabric-skill, MCP): TS unit tests with mocked pi runtime.

**Integration:**

- Per-agent-type smoke test: spin up auth-proxy with a mock OpenAI server (returns canned tool-use sequences), run a real Job container against a fixture issue/PR/transcript, assert correct output destination.
- Auth-proxy: real device-flow login against a mock OpenAI auth endpoint, verify token persistence across pod restart (PVC + restart test).
- TickTick MCP: integration test against a real TickTick sandbox account (premium-tier feature flag).

**End-to-end (Chainsaw, on a kind cluster):**

- Drop a fixture GitHub-issue webhook → verify PR opens within 10 min (with mock OpenAI responses).
- Drop a fixture TickTick meeting webhook → verify Obsidian vault writes the note.
- Trigger a fake Grafana alert → verify ops PR opens.

**Coverage floor:** 80% per service (Diixtra default), enforced in CI from day one for new services (auth-proxy, MCP servers). Existing services retain their current coverage during migration but cannot regress.

**No mocking of services we own.** Auth-proxy integration tests use a real auth-proxy + mock OpenAI; operator tests use real operator + envtest. Mocks only for OpenAI, GitHub, TickTick, Exa, Slack APIs.

## Migration plan

Nothing breaks during transition. Old and new run side-by-side gated by `AgentConfig.engine: claude-sdk | pi`.

**Phase 0 — auth-proxy lands with phone-driven reauth (1-2 days)**

- Build, deploy, login once.
- Verify Slack DM reauth flow end-to-end from phone.
- Verify weekly silent refresh works.
- No production impact — service exists but is dormant.

**Phase 1 — code-pr agent rebuilt on pi (~1 week)**

- New `runtime/src/agents/code-pr.ts` entrypoint, consuming the auth-proxy already deployed in Phase 0.
- Pi extensions: sandbox, slack-thread, MCP, fabric-skill (initial set, sufficient for code-pr).
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
| OpenAI subscription concurrency cap lower than expected | Medium | Throughput pain | Auth-proxy concurrency cap is configurable; priority queue absorbs the limit |
| Refresh token revoked by OpenAI policy change | Low | All agents paused until reauth | Slack DM reauth flow is fast (~5s on phone); weekly silent refresh keeps chain alive |
| Pi extension API changes (pre-1.0) | Medium | Local breakage on upgrade | Pin pi version; upgrade behind a feature flag |
| Cost premise wrong (subscription not actually cheaper) | Low | Pivot wasted effort | Phase 1 produces real comparison data before full commitment |
| Headless auth in K8s Job complexity underestimated | Low | Phase 0 grows | Phase 0 is dedicated to this exact problem; no agents touch it until proven |

## Open questions deferred to implementation

- Exact TickTick MCP tool surface — resolved in Phase 4 spike.
- Pi version pin — latest stable at Phase 0 start; revisit before Phase 1.
- Whether `AgentConfig` should be a ConfigMap or a new CRD — start with ConfigMap; promote to CRD only if dynamic config or status is needed.
- Auth-proxy on multi-replica vs single-pod — start single-pod (auth state is hard to share); promote to leader-elected if availability becomes an issue.

## Success criteria

1. All 6 agent types ship and run successfully on real tasks for at least 2 weeks each.
2. Monthly inference cost is measurably lower than the pre-pivot Anthropic API spend.
3. Manual reauth events are <1/month on average and resolved <1 minute on phone.
4. `ticktick-sync/` is removed; voice gateway preserved.
5. Test coverage ≥80% on auth-proxy, operator changes, and new MCP servers.
6. No regression in PR quality between SDK and pi-based code-pr agent (judged on PRs from a 2-week side-by-side comparison).
