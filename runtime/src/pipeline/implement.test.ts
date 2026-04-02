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
});
