# Paperless-ngx Integration & Agent SDK — Design Spec

**Date:** 2026-03-30
**Status:** Draft
**Author:** James + Claude
**Depends on:** AIOS Platform Agent Design (2026-03-27), AIOS Vault Redesign (2026-03-16)

---

## Purpose

Two pieces of work that complete the AIOS platform's core functionality:

1. **Paperless-ngx integration** — when a document is consumed by Paperless, automatically classify it via LocalAI, create a searchable stub note in the Obsidian vault, and link it to related existing notes.

2. **Agent SDK wiring** — implement the stubbed `implement.ts` pipeline stage so the AIOS coding agent can actually generate code using Claude.

---

## Piece 1: Paperless-ngx Integration

### Overview

Paperless-ngx is already deployed on Forge in the `paperless` namespace (`paperless-paperless-ngx.paperless.svc:8000`, external at `paperless.lab.kazie.co.uk`). This integration connects it to the AIOS ecosystem without building any Obsidian plugins — instead, it creates vault stub notes that Omnisearch indexes automatically.

### Architecture

```
Paperless-ngx (paperless namespace)
  │
  │ Document consumed → Workflow fires webhook
  │
  ▼ POST /webhook/paperless
aios-webhook (aios namespace)
  │
  ├─► Paperless API: fetch document metadata + OCR text
  │     GET paperless-paperless-ngx.paperless.svc:8000/api/documents/{id}/
  │
  ├─► LocalAI: classify document, suggest tags
  │     POST local-ai.local-ai.svc:8080/v1/chat/completions
  │
  ├─► Paperless API: apply tags
  │     PATCH paperless-paperless-ngx.paperless.svc:8000/api/documents/{id}/
  │
  ├─► Vault: write stub note (via Syncthing-mounted path on Forge)
  │     Knowledge/Paperless/{correspondent}/{title}.md
  │
  ├─► aios-search: find related vault notes
  │     POST aios-search.aios.svc:8080/search
  │
  └─► Vault: update related notes' frontmatter with paperless link
```

### Webhook Route

New route in aios-webhook: `POST /webhook/paperless`

The Paperless workflow sends a webhook on document consumption. The payload includes fields like `document_id`, `file_name`, `correspondent`, `tags`, `download_url`, and `original_filename`. The handler uses `document_id` to fetch full details. The handler:

1. Validates the request (shared secret, same pattern as GitHub webhook)
2. Fetches document details from the Paperless API (title, correspondent, tags, created date, full OCR content)
3. Calls LocalAI to classify the document and suggest additional tags
4. Applies suggested tags back to Paperless via its API
5. Writes a stub note to the vault
6. Queries aios-search for semantically related vault notes
7. For each related note above a similarity threshold, appends a `paperless` entry to the note's frontmatter

### Paperless API Client

New internal package: `internal/paperless/client.go`

```go
type Client struct {
    baseURL string    // paperless-paperless-ngx.paperless.svc:8000
    token   string    // API token from K8s secret
}

// GetDocument fetches document metadata and OCR content
func (c *Client) GetDocument(ctx context.Context, id int) (*Document, error)

// UpdateTags sets tags on a document
func (c *Client) UpdateTags(ctx context.Context, id int, tagIDs []int) error

// GetOrCreateTag finds a tag by name or creates it
func (c *Client) GetOrCreateTag(ctx context.Context, name string) (int, error)
```

The Paperless API uses token auth (`Authorization: Token <token>`). The token is stored as a 1Password-injected K8s secret.

### Document Struct

```go
type Document struct {
    ID             int       `json:"id"`
    Title          string    `json:"title"`
    Content        string    `json:"content"`         // OCR text
    Correspondent  string    `json:"correspondent"`   // resolved name
    Tags           []string  `json:"tags"`            // resolved names
    Created        time.Time `json:"created"`
    Added          time.Time `json:"added"`
    OriginalURL    string    // computed: https://paperless.lab.kazie.co.uk/documents/{id}
}
```

### LocalAI Auto-Tagging

New internal package: `internal/localai/client.go`

Calls LocalAI's OpenAI-compatible chat completions endpoint to classify the document. Uses the default model configured in LocalAI (currently `phi-3-mini` or equivalent small model — classification doesn't need a large model). The prompt includes the document title, correspondent, and first ~2000 characters of OCR text. The model returns a JSON array of suggested tag names. The model name is configurable via the `LOCALAI_MODEL` env var.

```go
type Client struct {
    baseURL string    // local-ai.local-ai.svc:8080
}

// ClassifyDocument returns suggested tags for a document
func (c *Client) ClassifyDocument(ctx context.Context, doc *paperless.Document) ([]string, error)
```

System prompt for classification:

```
You are a document classifier. Given the document metadata and content below,
return a JSON array of lowercase tag names that describe this document.
Use specific, consistent tags like: invoice, receipt, tax, contract, letter,
bank-statement, insurance, medical, utility-bill, payslip.
Return only the JSON array, no other text.
```

The handler takes the returned tags, resolves them to Paperless tag IDs (creating new tags if needed via `GetOrCreateTag`), and patches the document.

### Stub Note Format

Written to: `{vault_path}/Knowledge/Paperless/{correspondent}/{title}.md`

Where `{vault_path}` is the Syncthing-mounted vault directory on Forge (configured via env var `VAULT_PATH`).

```yaml
---
title: Self Assessment Tax Return 2024-25
type: paperless-document
source: paperless
paperless_id: 847
paperless_url: https://paperless.lab.kazie.co.uk/documents/847
correspondent: HMRC
tags: [tax, self-assessment]
created: 2025-01-15
added: 2026-03-30
entity: []
status: active
---

# Self Assessment Tax Return 2024-25

[View in Paperless](https://paperless.lab.kazie.co.uk/documents/847)

## Content

[full OCR text from Paperless]
```

The filename is sanitised (spaces → hyphens, lowercase, special chars removed, max 100 chars). If a stub already exists for the same `paperless_id`, it is overwritten (idempotent).

### Auto-Linking

After writing the stub, the handler queries `aios-search` for related vault notes:

```
POST aios-search.aios.svc:8080/search
{
  "query": "{title} {correspondent} {tags joined by space}",
  "top_k": 5,
  "min_score": 0.7
}
```

For each result above the similarity threshold:
- Read the note file from disk
- Parse YAML frontmatter
- If `paperless` key exists (list), append the URL if not already present
- If `paperless` key doesn't exist, add it as a new list field
- Write the file back

The `paperless` frontmatter field is always a list:

```yaml
paperless:
  - https://paperless.lab.kazie.co.uk/documents/847
  - https://paperless.lab.kazie.co.uk/documents/923
```

Notes in `Knowledge/Paperless/` are excluded from auto-link results (no self-linking).

### Paperless Workflow Configuration

Configured in the Paperless UI (not in code):
- **Trigger:** Consumption finished
- **Action:** Webhook to `http://aios-webhook.aios.svc:8080/webhook/paperless`
- **Headers:** `X-Paperless-Secret: <shared-secret>`

### K8s Manifest Changes

1. **OnePasswordItem** for Paperless API token (`aios/paperless-api-token`)
2. **OnePasswordItem** for Paperless webhook secret (`aios/paperless-webhook-secret`) — or reuse an existing secret with a new key
3. **NetworkPolicy** allowing `paperless` namespace → `aios-webhook` on port 8080
4. **aios-webhook Deployment** updates:
   - New env vars: `PAPERLESS_API_URL`, `PAPERLESS_API_TOKEN`, `PAPERLESS_WEBHOOK_SECRET`, `LOCALAI_URL`, `VAULT_PATH`, `PAPERLESS_DOMAIN`, `AIOS_SEARCH_URL`
   - Volume mount for the vault path (Syncthing directory on Forge)

### Error Handling

- If Paperless API is unreachable: log error, return 502. Paperless will retry the webhook.
- If LocalAI is unreachable: skip auto-tagging, continue with stub creation and auto-linking. Log warning.
- If vault write fails: log error, return 500. Paperless will retry.
- If aios-search is unreachable: skip auto-linking, stub is still created. Log warning.
- If frontmatter parsing fails on a related note: skip that note, continue with others. Log warning.

Auto-tagging and auto-linking are best-effort — failure in either should not prevent the stub note from being created.

---

## Piece 2: Agent SDK Integration

### Overview

The runtime pipeline is fully implemented except for `implement.ts`, which is a stub. All supporting infrastructure — sandbox, fabric, GitHub, memory, Slack, verify, deliver — is production-ready. The `@anthropic-ai/sdk` package is already in `package.json` (v0.80.0) but unused.

### What implement.ts Does

Takes the plan from the plan stage and uses Claude to generate code changes in the workspace. The executor calls it with:

```typescript
runImplement(config, slack, threadTs, plan, sandbox)
```

On verify failure, it's called again with the plan + verification feedback appended.

### Implementation Design

```typescript
import Anthropic from "@anthropic-ai/sdk";

export async function runImplement(
  config: TaskConfig,
  slack: SlackNotifier,
  threadTs: string,
  plan: string,
  sandbox: Sandbox,
): Promise<ImplementResult> {
  const client = new Anthropic();

  // Build system prompt
  const systemPrompt = buildSystemPrompt(config, plan);

  // Create initial messages with the implementation plan
  const messages: Anthropic.MessageParam[] = [
    { role: "user", content: plan },
  ];

  // Agent loop
  let response = await client.messages.create({
    model: config.model ?? "claude-sonnet-4-6",
    max_tokens: config.maxTokens ?? 16384,
    system: systemPrompt,
    messages,
    tools: buildToolDefinitions(),
  });

  // Process tool calls in a loop until the agent is done
  while (response.stop_reason === "tool_use") {
    const toolResults = await executeToolCalls(response, sandbox, config);
    messages.push({ role: "assistant", content: response.content });
    messages.push({ role: "user", content: toolResults });

    response = await client.messages.create({
      model: config.model ?? "claude-sonnet-4-6",
      max_tokens: config.maxTokens ?? 16384,
      system: systemPrompt,
      messages,
      tools: buildToolDefinitions(),
    });
  }

  // Stage and commit changes
  await stageAndCommit(sandbox, config);

  return { success: true, summary: extractSummary(response) };
}
```

### System Prompt

The system prompt provides the agent with:

1. **Role:** "You are a coding agent. Implement the plan below in the workspace."
2. **Workspace path:** `config.workspace` (the cloned repo in `/workspace`)
3. **Repository:** `config.repo`
4. **Constraints:** Only modify files within the workspace. Run tests after making changes. Follow existing code patterns.
5. **Plan:** The full plan text from the plan stage

### Tool Definitions

Three tools exposed to the Claude agent, all enforced through the existing `Sandbox`:

**`shell`** — Execute a shell command
```typescript
{
  name: "shell",
  description: "Execute a command in the workspace",
  input_schema: {
    type: "object",
    properties: {
      command: { type: "string", description: "The command to execute" },
      args: { type: "array", items: { type: "string" } },
    },
    required: ["command"],
  },
}
```

Execution: splits into command + args, passes through `sandboxedExec(sandbox, command, args, config.workspace)`. Returns stdout/stderr/exitCode.

**`read_file`** — Read a file from the workspace
```typescript
{
  name: "read_file",
  description: "Read file contents",
  input_schema: {
    type: "object",
    properties: {
      path: { type: "string", description: "Absolute file path" },
    },
    required: ["path"],
  },
}
```

Execution: validates path via `sandbox.validateFileAccess(path, "read")`, then reads with `fs.readFile`.

**`write_file`** — Write a file to the workspace
```typescript
{
  name: "write_file",
  description: "Write content to a file",
  input_schema: {
    type: "object",
    properties: {
      path: { type: "string", description: "Absolute file path" },
      content: { type: "string", description: "File content to write" },
    },
    required: ["path", "content"],
  },
}
```

Execution: validates path via `sandbox.validateFileAccess(path, "write")`, creates parent directories if needed, then writes with `fs.writeFile`.

### Tool Call Execution

```typescript
async function executeToolCalls(
  response: Anthropic.Message,
  sandbox: Sandbox,
  config: TaskConfig,
): Promise<Anthropic.ToolResultBlockParam[]> {
  const toolUseBlocks = response.content.filter(
    (block) => block.type === "tool_use",
  );

  const results: Anthropic.ToolResultBlockParam[] = [];

  for (const block of toolUseBlocks) {
    const result = await executeTool(block, sandbox, config);
    results.push({
      type: "tool_result",
      tool_use_id: block.id,
      content: result,
    });
  }

  return results;
}
```

Each tool call is executed sequentially (not in parallel) to maintain consistent workspace state.

### Stage and Commit

After the agent loop completes:

```typescript
async function stageAndCommit(sandbox: Sandbox, config: TaskConfig) {
  await sandboxedExec(sandbox, "git", ["add", "-A"], config.workspace);
  await sandboxedExec(
    sandbox,
    "git",
    ["commit", "-m", `aios: implement ${config.taskId}`],
    config.workspace,
  );
}
```

### Config Extension

`TaskConfig` needs two new optional fields for Agent SDK configuration:

```typescript
export interface TaskConfig {
  // ... existing fields ...
  /** Claude model to use (default: claude-sonnet-4-6) */
  model?: string;
  /** Max tokens for Claude responses (default: 16384) */
  maxTokens?: number;
}
```

These are populated from the `AgentConfig` CR's `spec.runtime.model` and `spec.runtime.maxTokens` fields, which already exist in the CRD spec.

### Slack Progress Updates

During the agent loop, post progress to the Slack thread when tool calls are made:

- On `shell` tool: `:terminal: Running: {command}`
- On `write_file` tool: `:pencil2: Writing: {path}`
- Throttled to max 1 message per 10 seconds to avoid Slack rate limits

### Error Handling

- If Claude API returns an error: propagate as `ImplementResult { success: false, summary: error.message }`
- If a tool call fails (sandbox blocks it): return the error message as the tool result to Claude — let it adapt
- If the agent loop exceeds a turn limit (50 turns): force-stop and return failure
- The executor's retry logic handles re-running implement with verify feedback on failure

### Testing

- Mock `@anthropic-ai/sdk` client in tests
- Test tool call routing (shell/read/write)
- Test sandbox enforcement (blocked commands return error to agent)
- Test stage-and-commit flow
- Test turn limit enforcement
- Existing executor tests already cover the retry loop with implement

---

## Observability

### New Metrics

| Metric | Type | Labels |
|--------|------|--------|
| `aios_paperless_documents_processed_total` | counter | `status` (success/error) |
| `aios_paperless_tags_applied_total` | counter | — |
| `aios_paperless_stubs_created_total` | counter | — |
| `aios_paperless_autolinks_created_total` | counter | — |
| `aios_paperless_localai_duration_seconds` | histogram | — |
| `aios_implement_tool_calls_total` | counter | `tool` (shell/read/write), `allowed` |
| `aios_implement_turns_total` | counter | `task_id` |
| `aios_implement_duration_seconds` | histogram | — |

### Structured Logs

Paperless handler logs include: `document_id`, `correspondent`, `tags_applied`, `stub_path`, `autolinks_count`.

Implement stage logs include: `task_id`, `tool`, `command` (for shell), `path` (for file ops), `turn_number`.

---

## What's Not Being Built

- **Obsidian plugin** — stub notes + Omnisearch handles search natively
- **aios-search extension** — stubs are vault notes, already indexed
- **Backfill** — Paperless is empty, forward-only
- **Bidirectional tag sync** — tags flow from Paperless → vault stubs, not back
- **Complex AI document analysis** — future spec using Agent SDK for multi-step reasoning
- **Multi-agent coordination** — separate future spec (A2A protocol)

---

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Stub notes instead of Obsidian plugin | Zero plugin maintenance. Omnisearch and aios-search index stubs for free. Works across all Obsidian devices via Syncthing. |
| LocalAI for auto-tagging instead of Claude | Zero marginal cost, sub-second latency, already deployed on Forge. Document classification doesn't need Claude-level reasoning. |
| Webhook handler does everything synchronously | Document ingest is low-volume (a few per day). No need for async queues or separate workers. Keeps architecture simple. |
| Auto-link via aios-search similarity | Reuses existing semantic search infrastructure. No custom matching logic needed. |
| Full OCR text in stub body | Enables Omnisearch full-text search across Paperless documents. Documents are mostly single-page, so size is manageable. |
| `paperless` frontmatter as list | A vault note (e.g. a project) can relate to multiple Paperless documents. |
| Correspondent subfolders | Mirrors Paperless filename format. Natural organisation for browsing. |
| Sequential tool execution in implement.ts | Parallel execution could cause race conditions in the workspace (two writes to same file). Sequential is safer and simpler. |
| Three tools (shell/read/write) | Minimal surface area. Shell covers build/test/git. Read/write cover file ops. No need for search/grep tools — agent uses shell for those. |
| 50-turn limit | Safety valve. A well-functioning agent should complete in 10-20 turns. 50 allows for complex tasks without infinite loops. |
