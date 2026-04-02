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

// Mock sandboxedExec for git operations
vi.mock("../sandbox.js", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../sandbox.js")>();
  return {
    ...actual,
    sandboxedExec: vi.fn().mockResolvedValue({ exitCode: 0, stdout: "", stderr: "" }),
  };
});

import Anthropic from "@anthropic-ai/sdk";
import { executeTool } from "../agent-tools.js";
import { sandboxedExec } from "../sandbox.js";

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
    // Default: git add succeeds, git diff --cached has changes (exit 1), git commit succeeds
    vi.mocked(sandboxedExec)
      .mockResolvedValueOnce({ exitCode: 0, stdout: "", stderr: "" })  // git add
      .mockResolvedValueOnce({ exitCode: 1, stdout: "", stderr: "" })  // git diff --cached --quiet (has changes)
      .mockResolvedValueOnce({ exitCode: 0, stdout: "", stderr: "" }); // git commit
  });

  it("completes successfully when agent returns end_turn", async () => {
    const mockCreate = vi.fn().mockResolvedValue({
      stop_reason: "end_turn",
      content: [
        { type: "text", text: "I've implemented the changes." },
      ],
    });
    vi.mocked(Anthropic).mockImplementation(function () {
      return { messages: { create: mockCreate } } as any;
    });

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
    // Verify git operations were called
    expect(sandboxedExec).toHaveBeenCalledWith(sandbox, "git", ["add", "-A"], "/workspace");
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
    vi.mocked(Anthropic).mockImplementation(function () {
      return { messages: { create: mockCreate } } as any;
    });

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
    vi.mocked(Anthropic).mockImplementation(function () {
      return { messages: { create: mockCreate } } as any;
    });

    const result = await runImplement(
      mockConfig,
      mockSlack,
      "thread-1",
      "Fix the bug",
      sandbox,
    );

    expect(result.success).toBe(false);
    expect(result.summary).toContain("turn limit");
    // 1 initial + 50 loop iterations = 51 calls max
    expect(mockCreate.mock.calls.length).toBeLessThanOrEqual(51);
  });

  it("handles API error gracefully", async () => {
    const mockCreate = vi.fn().mockRejectedValue(new Error("API rate limit"));
    vi.mocked(Anthropic).mockImplementation(function () {
      return { messages: { create: mockCreate } } as any;
    });

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

  it("returns failure when git add fails", async () => {
    vi.mocked(sandboxedExec)
      .mockReset()
      .mockResolvedValueOnce({ exitCode: 126, stdout: "", stderr: "Sandbox blocked command" });

    const mockCreate = vi.fn().mockResolvedValue({
      stop_reason: "end_turn",
      content: [{ type: "text", text: "Done." }],
    });
    vi.mocked(Anthropic).mockImplementation(function () {
      return { messages: { create: mockCreate } } as any;
    });

    const result = await runImplement(
      mockConfig,
      mockSlack,
      "thread-1",
      "Fix bug",
      sandbox,
    );

    expect(result.success).toBe(false);
    expect(result.summary).toContain("Git commit failed");
  });

  it("returns failure when git commit fails", async () => {
    vi.mocked(sandboxedExec)
      .mockReset()
      .mockResolvedValueOnce({ exitCode: 0, stdout: "", stderr: "" })   // git add
      .mockResolvedValueOnce({ exitCode: 1, stdout: "", stderr: "" })   // git diff --cached (has changes)
      .mockResolvedValueOnce({ exitCode: 1, stdout: "", stderr: "nothing to commit" }); // git commit

    const mockCreate = vi.fn().mockResolvedValue({
      stop_reason: "end_turn",
      content: [{ type: "text", text: "Done." }],
    });
    vi.mocked(Anthropic).mockImplementation(function () {
      return { messages: { create: mockCreate } } as any;
    });

    const result = await runImplement(
      mockConfig,
      mockSlack,
      "thread-1",
      "Fix bug",
      sandbox,
    );

    expect(result.success).toBe(false);
    expect(result.summary).toContain("nothing to commit");
  });

  it("succeeds with no commit when there are no staged changes", async () => {
    vi.mocked(sandboxedExec)
      .mockReset()
      .mockResolvedValueOnce({ exitCode: 0, stdout: "", stderr: "" })  // git add
      .mockResolvedValueOnce({ exitCode: 0, stdout: "", stderr: "" }); // git diff --cached --quiet (no changes)

    const mockCreate = vi.fn().mockResolvedValue({
      stop_reason: "end_turn",
      content: [{ type: "text", text: "No changes needed." }],
    });
    vi.mocked(Anthropic).mockImplementation(function () {
      return { messages: { create: mockCreate } } as any;
    });

    const result = await runImplement(
      mockConfig,
      mockSlack,
      "thread-1",
      "Fix bug",
      sandbox,
    );

    expect(result.success).toBe(true);
    // git commit should not have been called (only 2 sandboxedExec calls: add + diff)
    expect(sandboxedExec).toHaveBeenCalledTimes(2);
  });

  it("sanitizes taskId in commit message", async () => {
    const configWithHash = { ...mockConfig, taskId: "task#42" };

    const mockCreate = vi.fn().mockResolvedValue({
      stop_reason: "end_turn",
      content: [{ type: "text", text: "Done." }],
    });
    vi.mocked(Anthropic).mockImplementation(function () {
      return { messages: { create: mockCreate } } as any;
    });

    await runImplement(
      configWithHash,
      mockSlack,
      "thread-1",
      "Fix bug",
      sandbox,
    );

    // The commit call should have sanitized the # character
    const commitCall = vi.mocked(sandboxedExec).mock.calls.find(
      (call) => call[1] === "git" && call[2][0] === "commit",
    );
    expect(commitCall).toBeDefined();
    expect(commitCall![2][2]).toBe("aios: implement task_42");
  });
});
