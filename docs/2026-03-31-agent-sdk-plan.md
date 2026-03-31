# Agent SDK Integration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the stubbed `implement.ts` pipeline stage to use the Claude Agent SDK (`@anthropic-ai/sdk`) so the AIOS coding agent can actually generate code.

**Architecture:** Replace the no-op stub in `implement.ts` with a tool-use agentic loop. Claude receives the plan, uses three sandboxed tools (shell, read_file, write_file), and iterates until implementation is complete. All commands go through the existing `Sandbox` class. Progress is posted to Slack.

**Tech Stack:** TypeScript, `@anthropic-ai/sdk` 0.80.0, Vitest, existing Sandbox/Slack infrastructure

---

### Task 1: Extend TaskConfig with Model and MaxTokens

**Files:**
- Modify: `runtime/src/types.ts`
- Modify: `runtime/src/config.ts`
- Modify: `runtime/src/config.test.ts`

- [ ] **Step 1: Write the failing test for new config fields**

Add to `runtime/src/config.test.ts`, in the `loadTaskConfig` describe block:

```typescript
it("loads model and maxTokens from env", () => {
  process.env.TASK_ID = "test-1";
  process.env.TASK_TYPE = "code";
  process.env.TASK_PROMPT = "fix bug";
  process.env.TASK_REPO = "Diixtra/aios";
  process.env.TASK_BRANCH = "main";
  process.env.SLACK_CHANNEL = "C123";
  process.env.MEMORY_URL = "http://memory:8080";
  process.env.SEARCH_URL = "http://search:8080";
  process.env.WORKSPACE = "/workspace";
  process.env.CLAUDE_MODEL = "claude-sonnet-4-6";
  process.env.CLAUDE_MAX_TOKENS = "32768";

  const config = loadTaskConfig();
  expect(config.model).toBe("claude-sonnet-4-6");
  expect(config.maxTokens).toBe(32768);
});

it("uses defaults for model and maxTokens when not set", () => {
  process.env.TASK_ID = "test-1";
  process.env.TASK_TYPE = "code";
  process.env.TASK_PROMPT = "fix bug";
  process.env.TASK_REPO = "Diixtra/aios";
  process.env.TASK_BRANCH = "main";
  process.env.SLACK_CHANNEL = "C123";
  process.env.MEMORY_URL = "http://memory:8080";
  process.env.SEARCH_URL = "http://search:8080";
  process.env.WORKSPACE = "/workspace";

  const config = loadTaskConfig();
  expect(config.model).toBe("claude-sonnet-4-6");
  expect(config.maxTokens).toBe(16384);
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd runtime && npx vitest run src/config.test.ts`
Expected: FAIL — `model` and `maxTokens` not in config

- [ ] **Step 3: Add fields to TaskConfig**

In `runtime/src/types.ts`, add to the `TaskConfig` interface:

```typescript
  /** Claude model to use (default: claude-sonnet-4-6) */
  model: string;
  /** Max tokens for Claude responses (default: 16384) */
  maxTokens: number;
```

- [ ] **Step 4: Update loadTaskConfig to read new env vars**

In `runtime/src/config.ts`, add to the `loadTaskConfig` function return object:

```typescript
    model: process.env.CLAUDE_MODEL ?? "claude-sonnet-4-6",
    maxTokens: process.env.CLAUDE_MAX_TOKENS
      ? parseInt(process.env.CLAUDE_MAX_TOKENS, 10)
      : 16384,
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd runtime && npx vitest run src/config.test.ts`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
cd runtime
git add src/types.ts src/config.ts src/config.test.ts
git commit -m "feat(runtime): add model and maxTokens to TaskConfig

Read from CLAUDE_MODEL and CLAUDE_MAX_TOKENS env vars.
Defaults: claude-sonnet-4-6, 16384."
```

---

### Task 2: Implement the Agent Tool Definitions

**Files:**
- Create: `runtime/src/agent-tools.ts`
- Create: `runtime/src/agent-tools.test.ts`

- [ ] **Step 1: Write the failing test for tool execution**

Create `runtime/src/agent-tools.test.ts`:

```typescript
import { describe, it, expect, vi, beforeEach } from "vitest";
import { buildToolDefinitions, executeTool } from "./agent-tools.js";
import { Sandbox } from "./sandbox.js";
import type { ToolPolicy } from "./types.js";
import * as fs from "node:fs/promises";

vi.mock("node:fs/promises");

const testPolicy: ToolPolicy = {
  allowedCommands: ["ls ", "cat ", "git "],
  deniedCommands: ["rm -rf"],
  writablePaths: ["/workspace/**"],
  readablePaths: ["/workspace/**"],
};

describe("buildToolDefinitions", () => {
  it("returns three tool definitions", () => {
    const tools = buildToolDefinitions();
    expect(tools).toHaveLength(3);
    expect(tools.map((t) => t.name)).toEqual(["shell", "read_file", "write_file"]);
  });
});

describe("executeTool", () => {
  let sandbox: Sandbox;

  beforeEach(() => {
    sandbox = new Sandbox(testPolicy);
    vi.clearAllMocks();
  });

  it("executes allowed shell command", async () => {
    const result = await executeTool(
      {
        type: "tool_use",
        id: "tool-1",
        name: "shell",
        input: { command: "ls", args: ["-la"] },
      },
      sandbox,
      "/workspace",
    );

    // sandboxedExec will run the actual command — in test env it may fail
    // but we verify the result has the expected shape
    expect(result).toHaveProperty("content");
  });

  it("returns error for blocked shell command", async () => {
    const result = await executeTool(
      {
        type: "tool_use",
        id: "tool-2",
        name: "shell",
        input: { command: "rm -rf", args: ["/"] },
      },
      sandbox,
      "/workspace",
    );

    expect(result.content).toContain("Sandbox blocked");
  });

  it("reads file within allowed path", async () => {
    vi.mocked(fs.readFile).mockResolvedValue("file contents");

    const result = await executeTool(
      {
        type: "tool_use",
        id: "tool-3",
        name: "read_file",
        input: { path: "/workspace/src/index.ts" },
      },
      sandbox,
      "/workspace",
    );

    expect(result.content).toBe("file contents");
    expect(fs.readFile).toHaveBeenCalledWith("/workspace/src/index.ts", "utf-8");
  });

  it("blocks read outside allowed path", async () => {
    const result = await executeTool(
      {
        type: "tool_use",
        id: "tool-4",
        name: "read_file",
        input: { path: "/etc/passwd" },
      },
      sandbox,
      "/workspace",
    );

    expect(result.content).toContain("does not match");
    expect(fs.readFile).not.toHaveBeenCalled();
  });

  it("writes file within allowed path", async () => {
    vi.mocked(fs.mkdir).mockResolvedValue(undefined);
    vi.mocked(fs.writeFile).mockResolvedValue();

    const result = await executeTool(
      {
        type: "tool_use",
        id: "tool-5",
        name: "write_file",
        input: { path: "/workspace/src/new.ts", content: "const x = 1;" },
      },
      sandbox,
      "/workspace",
    );

    expect(result.content).toContain("Written");
    expect(fs.writeFile).toHaveBeenCalledWith("/workspace/src/new.ts", "const x = 1;", "utf-8");
  });

  it("blocks write outside allowed path", async () => {
    const result = await executeTool(
      {
        type: "tool_use",
        id: "tool-6",
        name: "write_file",
        input: { path: "/etc/evil.sh", content: "bad" },
      },
      sandbox,
      "/workspace",
    );

    expect(result.content).toContain("does not match");
    expect(fs.writeFile).not.toHaveBeenCalled();
  });

  it("returns error for unknown tool", async () => {
    const result = await executeTool(
      {
        type: "tool_use",
        id: "tool-7",
        name: "unknown_tool",
        input: {},
      },
      sandbox,
      "/workspace",
    );

    expect(result.content).toContain("Unknown tool");
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd runtime && npx vitest run src/agent-tools.test.ts`
Expected: FAIL — module does not exist

- [ ] **Step 3: Implement agent-tools.ts**

Create `runtime/src/agent-tools.ts`:

```typescript
import type Anthropic from "@anthropic-ai/sdk";
import { Sandbox, sandboxedExec } from "./sandbox.js";
import * as fs from "node:fs/promises";
import * as path from "node:path";

export interface ToolCallBlock {
  type: "tool_use";
  id: string;
  name: string;
  input: Record<string, unknown>;
}

export interface ToolResult {
  content: string;
  is_error?: boolean;
}

/** Returns the three tool definitions for the Claude agent. */
export function buildToolDefinitions(): Anthropic.Tool[] {
  return [
    {
      name: "shell",
      description:
        "Execute a command in the workspace. Returns stdout, stderr, and exit code.",
      input_schema: {
        type: "object" as const,
        properties: {
          command: {
            type: "string",
            description: "The command to execute (e.g. 'git', 'npm', 'ls')",
          },
          args: {
            type: "array",
            items: { type: "string" },
            description: "Command arguments",
          },
        },
        required: ["command"],
      },
    },
    {
      name: "read_file",
      description: "Read the contents of a file.",
      input_schema: {
        type: "object" as const,
        properties: {
          path: {
            type: "string",
            description: "Absolute file path to read",
          },
        },
        required: ["path"],
      },
    },
    {
      name: "write_file",
      description: "Write content to a file. Creates parent directories if needed.",
      input_schema: {
        type: "object" as const,
        properties: {
          path: {
            type: "string",
            description: "Absolute file path to write",
          },
          content: {
            type: "string",
            description: "File content to write",
          },
        },
        required: ["path", "content"],
      },
    },
  ];
}

/** Executes a single tool call, enforcing sandbox policy. */
export async function executeTool(
  block: ToolCallBlock,
  sandbox: Sandbox,
  workspace: string,
): Promise<ToolResult> {
  switch (block.name) {
    case "shell":
      return executeShell(block.input, sandbox, workspace);
    case "read_file":
      return executeReadFile(block.input, sandbox);
    case "write_file":
      return executeWriteFile(block.input, sandbox);
    default:
      return { content: `Unknown tool: ${block.name}`, is_error: true };
  }
}

async function executeShell(
  input: Record<string, unknown>,
  sandbox: Sandbox,
  workspace: string,
): Promise<ToolResult> {
  const command = input.command as string;
  const args = (input.args as string[]) ?? [];

  const result = await sandboxedExec(sandbox, command, args, workspace);

  if (result.exitCode === 126) {
    // Sandbox blocked the command
    return { content: result.stderr, is_error: true };
  }

  const output = [
    `Exit code: ${result.exitCode}`,
    result.stdout ? `stdout:\n${result.stdout}` : "",
    result.stderr ? `stderr:\n${result.stderr}` : "",
  ]
    .filter(Boolean)
    .join("\n");

  return { content: output };
}

async function executeReadFile(
  input: Record<string, unknown>,
  sandbox: Sandbox,
): Promise<ToolResult> {
  const filePath = input.path as string;

  const validation = sandbox.validateFileAccess(filePath, "read");
  if (!validation.allowed) {
    return { content: `Read blocked: ${validation.reason}`, is_error: true };
  }

  try {
    const content = await fs.readFile(filePath, "utf-8");
    return { content };
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    return { content: `Read error: ${message}`, is_error: true };
  }
}

async function executeWriteFile(
  input: Record<string, unknown>,
  sandbox: Sandbox,
): Promise<ToolResult> {
  const filePath = input.path as string;
  const content = input.content as string;

  const validation = sandbox.validateFileAccess(filePath, "write");
  if (!validation.allowed) {
    return { content: `Write blocked: ${validation.reason}`, is_error: true };
  }

  try {
    await fs.mkdir(path.dirname(filePath), { recursive: true });
    await fs.writeFile(filePath, content, "utf-8");
    return { content: `Written: ${filePath}` };
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    return { content: `Write error: ${message}`, is_error: true };
  }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd runtime && npx vitest run src/agent-tools.test.ts`
Expected: PASS (all tests)

- [ ] **Step 5: Commit**

```bash
cd runtime
git add src/agent-tools.ts src/agent-tools.test.ts
git commit -m "feat(runtime): add agent tool definitions and executor

Three sandboxed tools: shell, read_file, write_file.
All validated against ToolPolicy before execution."
```

---

### Task 3: Implement the Agent Loop in implement.ts

**Files:**
- Modify: `runtime/src/pipeline/implement.ts`
- Modify: `runtime/src/pipeline/implement.test.ts`

- [ ] **Step 1: Write failing tests for the new implementation**

Replace the contents of `runtime/src/pipeline/implement.test.ts`:

```typescript
import { describe, it, expect, vi, beforeEach } from "vitest";
import { runImplement } from "./implement.js";
import type { TaskConfig } from "../types.js";
import type { SlackNotifier } from "../slack.js";
import { Sandbox } from "../sandbox.js";

// Mock the Anthropic SDK
vi.mock("@anthropic-ai/sdk", () => {
  return {
    default: vi.fn(),
  };
});

// Mock agent-tools
vi.mock("../agent-tools.js", () => ({
  buildToolDefinitions: vi.fn(() => [
    { name: "shell", description: "shell", input_schema: { type: "object", properties: {} } },
  ]),
  executeTool: vi.fn(() => Promise.resolve({ content: "ok" })),
}));

import Anthropic from "@anthropic-ai/sdk";
import { executeTool } from "../agent-tools.js";

const mockConfig: TaskConfig = {
  taskId: "test-1",
  taskType: "code",
  prompt: "fix the bug",
  repo: "Diixtra/aios",
  branch: "main",
  slackChannel: "C123",
  memoryUrl: "http://memory:8080",
  searchUrl: "http://search:8080",
  workspace: "/workspace",
  model: "claude-sonnet-4-6",
  maxTokens: 16384,
};

const mockSlack = {
  postToThread: vi.fn().mockResolvedValue(undefined),
} as unknown as SlackNotifier;

const testPolicy = {
  allowedCommands: ["git "],
  deniedCommands: [],
  writablePaths: ["/workspace/**"],
  readablePaths: ["/workspace/**"],
};

describe("runImplement", () => {
  let sandbox: Sandbox;

  beforeEach(() => {
    sandbox = new Sandbox(testPolicy);
    vi.clearAllMocks();
  });

  it("completes successfully when agent returns end_turn", async () => {
    const mockCreate = vi.fn().mockResolvedValue({
      stop_reason: "end_turn",
      content: [
        { type: "text", text: "I've implemented the changes." },
      ],
    });
    vi.mocked(Anthropic).mockImplementation(
      () => ({ messages: { create: mockCreate } }) as any,
    );

    const result = await runImplement(
      mockConfig,
      mockSlack,
      "thread-1",
      "## Plan\nFix the bug in auth.ts",
      sandbox,
    );

    expect(result.success).toBe(true);
    expect(result.summary).toContain("implemented");
    expect(mockCreate).toHaveBeenCalledTimes(1);
    expect(mockSlack.postToThread).toHaveBeenCalled();
  });

  it("processes tool calls in a loop", async () => {
    const mockCreate = vi
      .fn()
      .mockResolvedValueOnce({
        stop_reason: "tool_use",
        content: [
          {
            type: "tool_use",
            id: "tool-1",
            name: "shell",
            input: { command: "ls", args: ["-la"] },
          },
        ],
      })
      .mockResolvedValueOnce({
        stop_reason: "end_turn",
        content: [{ type: "text", text: "Done implementing." }],
      });
    vi.mocked(Anthropic).mockImplementation(
      () => ({ messages: { create: mockCreate } }) as any,
    );

    const result = await runImplement(
      mockConfig,
      mockSlack,
      "thread-1",
      "Fix the bug",
      sandbox,
    );

    expect(result.success).toBe(true);
    expect(mockCreate).toHaveBeenCalledTimes(2);
    expect(executeTool).toHaveBeenCalledTimes(1);
  });

  it("enforces turn limit", async () => {
    const mockCreate = vi.fn().mockResolvedValue({
      stop_reason: "tool_use",
      content: [
        {
          type: "tool_use",
          id: "tool-1",
          name: "shell",
          input: { command: "ls" },
        },
      ],
    });
    vi.mocked(Anthropic).mockImplementation(
      () => ({ messages: { create: mockCreate } }) as any,
    );

    const result = await runImplement(
      mockConfig,
      mockSlack,
      "thread-1",
      "Fix the bug",
      sandbox,
    );

    expect(result.success).toBe(false);
    expect(result.summary).toContain("turn limit");
    // 50 turns + 1 initial = 51 calls max, but we check for 51
    expect(mockCreate.mock.calls.length).toBeLessThanOrEqual(51);
  });

  it("handles API error gracefully", async () => {
    const mockCreate = vi.fn().mockRejectedValue(new Error("API rate limit"));
    vi.mocked(Anthropic).mockImplementation(
      () => ({ messages: { create: mockCreate } }) as any,
    );

    const result = await runImplement(
      mockConfig,
      mockSlack,
      "thread-1",
      "Fix the bug",
      sandbox,
    );

    expect(result.success).toBe(false);
    expect(result.summary).toContain("API rate limit");
  });

  it("stages and commits after successful implementation", async () => {
    const mockCreate = vi.fn().mockResolvedValue({
      stop_reason: "end_turn",
      content: [{ type: "text", text: "Changes complete." }],
    });
    vi.mocked(Anthropic).mockImplementation(
      () => ({ messages: { create: mockCreate } }) as any,
    );

    // Track sandboxedExec calls for git add and git commit
    const { sandboxedExec } = await import("../sandbox.js");
    const execSpy = vi.spyOn(
      await import("../sandbox.js"),
      "sandboxedExec",
    );
    execSpy.mockResolvedValue({ exitCode: 0, stdout: "", stderr: "" });

    await runImplement(
      mockConfig,
      mockSlack,
      "thread-1",
      "Fix bug",
      sandbox,
    );

    const gitAddCall = execSpy.mock.calls.find(
      (call) => call[1] === "git" && call[2][0] === "add",
    );
    const gitCommitCall = execSpy.mock.calls.find(
      (call) => call[1] === "git" && call[2][0] === "commit",
    );

    expect(gitAddCall).toBeDefined();
    expect(gitCommitCall).toBeDefined();

    execSpy.mockRestore();
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd runtime && npx vitest run src/pipeline/implement.test.ts`
Expected: FAIL — implement.ts is still a stub

- [ ] **Step 3: Replace implement.ts with full implementation**

Replace the contents of `runtime/src/pipeline/implement.ts`:

```typescript
import Anthropic from "@anthropic-ai/sdk";
import type { TaskConfig } from "../types.js";
import type { SlackNotifier } from "../slack.js";
import { Sandbox, sandboxedExec } from "../sandbox.js";
import {
  buildToolDefinitions,
  executeTool,
  type ToolCallBlock,
} from "../agent-tools.js";

export interface ImplementResult {
  /** Whether implementation completed without errors */
  success: boolean;
  /** Summary of changes made */
  summary: string;
}

const MAX_TURNS = 50;
const SLACK_THROTTLE_MS = 10_000;

/**
 * Implement stage: executes the implementation plan using Claude Agent SDK.
 *
 * Runs a tool-use agentic loop where Claude reads code, writes changes,
 * and runs commands — all sandboxed via the ToolPolicy. Stages and commits
 * changes on success.
 */
export async function runImplement(
  config: TaskConfig,
  slack: SlackNotifier,
  threadTs: string,
  plan: string,
  sandbox: Sandbox,
): Promise<ImplementResult> {
  await slack.postToThread(
    config.slackChannel,
    threadTs,
    `:hammer_and_wrench: Starting implementation...`,
  );

  const client = new Anthropic();
  const systemPrompt = buildSystemPrompt(config);
  const tools = buildToolDefinitions();
  const messages: Anthropic.MessageParam[] = [
    { role: "user", content: plan },
  ];

  try {
    let response = await client.messages.create({
      model: config.model,
      max_tokens: config.maxTokens,
      system: systemPrompt,
      messages,
      tools,
    });

    let turns = 0;
    let lastSlackPost = 0;

    while (response.stop_reason === "tool_use" && turns < MAX_TURNS) {
      turns++;
      const toolBlocks = response.content.filter(
        (b): b is Anthropic.ToolUseBlock => b.type === "tool_use",
      );

      const toolResults: Anthropic.ToolResultBlockParam[] = [];

      for (const block of toolBlocks) {
        // Throttled Slack progress
        const now = Date.now();
        if (now - lastSlackPost > SLACK_THROTTLE_MS) {
          const label =
            block.name === "shell"
              ? `:terminal: Running: ${(block.input as any).command}`
              : block.name === "write_file"
                ? `:pencil2: Writing: ${(block.input as any).path}`
                : `:mag: Reading: ${(block.input as any).path}`;
          await slack.postToThread(config.slackChannel, threadTs, label);
          lastSlackPost = now;
        }

        const result = await executeTool(
          block as ToolCallBlock,
          sandbox,
          config.workspace,
        );

        toolResults.push({
          type: "tool_result",
          tool_use_id: block.id,
          content: result.content,
          is_error: result.is_error,
        });
      }

      messages.push({ role: "assistant", content: response.content });
      messages.push({ role: "user", content: toolResults });

      response = await client.messages.create({
        model: config.model,
        max_tokens: config.maxTokens,
        system: systemPrompt,
        messages,
        tools,
      });
    }

    if (turns >= MAX_TURNS) {
      await slack.postToThread(
        config.slackChannel,
        threadTs,
        `:warning: Implementation hit turn limit (${MAX_TURNS})`,
      );
      return {
        success: false,
        summary: `Implementation exceeded turn limit (${MAX_TURNS})`,
      };
    }

    // Stage and commit changes
    await sandboxedExec(sandbox, "git", ["add", "-A"], config.workspace);
    await sandboxedExec(
      sandbox,
      "git",
      ["commit", "-m", `aios: implement ${config.taskId}`],
      config.workspace,
    );

    const summary = extractSummary(response);

    await slack.postToThread(
      config.slackChannel,
      threadTs,
      `:white_check_mark: Implementation complete.`,
    );

    return { success: true, summary };
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    await slack.postToThread(
      config.slackChannel,
      threadTs,
      `:x: Implementation failed: ${message}`,
    );
    return { success: false, summary: message };
  }
}

function buildSystemPrompt(config: TaskConfig): string {
  return `You are a coding agent. Implement the plan provided by the user.

Workspace: ${config.workspace}
Repository: ${config.repo}

Rules:
- Only modify files within ${config.workspace}
- Run tests after making changes to verify correctness
- Follow existing code patterns and conventions
- Make small, focused changes
- If a command is blocked by the sandbox, find an alternative approach

You have three tools: shell (run commands), read_file, and write_file.`;
}

function extractSummary(response: Anthropic.Message): string {
  const textBlocks = response.content.filter(
    (b): b is Anthropic.TextBlock => b.type === "text",
  );
  if (textBlocks.length === 0) {
    return "Implementation completed (no summary provided)";
  }
  return textBlocks.map((b) => b.text).join("\n");
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd runtime && npx vitest run src/pipeline/implement.test.ts`
Expected: PASS (all tests)

- [ ] **Step 5: Run the full test suite**

Run: `cd runtime && npx vitest run`
Expected: PASS (all tests across all files)

- [ ] **Step 6: Commit**

```bash
cd runtime
git add src/pipeline/implement.ts src/pipeline/implement.test.ts
git commit -m "feat(runtime): wire implement.ts to Claude Agent SDK

Replaces the no-op stub with a full tool-use agentic loop.
Three sandboxed tools: shell, read_file, write_file.
50-turn safety limit, throttled Slack progress updates,
stages and commits on success."
```

---

### Task 4: Update Executor Tests

**Files:**
- Modify: `runtime/src/executor.test.ts`

- [ ] **Step 1: Verify existing executor tests still pass**

Run: `cd runtime && npx vitest run src/executor.test.ts`
Expected: PASS — executor tests mock `runImplement` and should be unaffected

If any tests fail due to the new `model`/`maxTokens` fields missing from test fixtures, update the mock configs to include:

```typescript
model: "claude-sonnet-4-6",
maxTokens: 16384,
```

- [ ] **Step 2: Fix any failing tests and commit if needed**

Run: `cd runtime && npx vitest run`
Expected: PASS (full suite)

```bash
cd runtime
git add -A
git commit -m "fix(runtime): update test fixtures for new TaskConfig fields"
```

Only commit if changes were needed.

---

### Task 5: Update Dockerfile and CI

**Files:**
- Verify: `runtime/Dockerfile` (should need no changes — deps already in package.json)
- Create: `.github/workflows/runtime-build.yaml` (if not exists)

- [ ] **Step 1: Check if runtime CI workflow exists**

Run: `ls .github/workflows/ | grep runtime`

If no runtime workflow exists, create `.github/workflows/runtime-build.yaml`:

```yaml
name: Build runtime

on:
  push:
    branches: [main]
    paths:
      - "runtime/**"
  pull_request:
    paths:
      - "runtime/**"

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: "24"
          cache: "npm"
          cache-dependency-path: runtime/package-lock.json
      - name: Install
        working-directory: runtime
        run: npm ci
      - name: Test
        working-directory: runtime
        run: npx vitest run --coverage

  build:
    needs: test
    if: github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    steps:
      - uses: actions/checkout@v4
      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - uses: docker/setup-buildx-action@v3
      - uses: docker/build-push-action@v6
        with:
          context: runtime
          push: true
          tags: ghcr.io/diixtra/aios-runtime:latest
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

- [ ] **Step 2: Build Docker image locally to verify**

Run: `cd runtime && docker build -t aios-runtime:test .`
Expected: Image builds successfully

- [ ] **Step 3: Commit if workflow was created**

```bash
git add .github/workflows/runtime-build.yaml
git commit -m "ci: add runtime build and test workflow"
```

---

### Task 6: Final Verification

- [ ] **Step 1: Run full runtime test suite with coverage**

Run: `cd runtime && npx vitest run --coverage`
Expected: PASS with >80% coverage

- [ ] **Step 2: Run full webhook test suite**

Run: `cd webhook && go test ./... -v -race -cover`
Expected: PASS with >80% coverage on new packages

- [ ] **Step 3: Verify no regressions across the repo**

Run both test suites and confirm everything passes.
