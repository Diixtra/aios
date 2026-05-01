import { describe, it, expect, vi, beforeEach } from "vitest";
import { runPlan } from "./plan.js";
import type { TaskConfig } from "../types.js";

const config: TaskConfig = {
  taskId: "task-1",
  taskType: "code",
  prompt: "Implement feature X",
  repo: "owner/repo",
  branch: "feat-x",
  slackChannel: "C123",
  memoryUrl: "http://memory:8080",
  searchUrl: "http://search:8080",
  workspace: "/workspace",
  model: "claude-sonnet-4-6",
  maxTokens: 16384,
};

const mockFabric = {
  run: vi.fn(),
};

const mockSlack = {
  postToThread: vi.fn().mockResolvedValue(undefined),
};

describe("runPlan", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockFabric.run.mockResolvedValue({
      exitCode: 0,
      stdout: "1. Parse input\n2. Transform\n3. Output",
      stderr: "",
    });
  });

  it("calls fabric with the plan pattern", async () => {
    await runPlan(config, mockFabric as any, mockSlack as any, "ts-1", "understanding text");

    expect(mockFabric.run).toHaveBeenCalledWith("plan", expect.stringContaining("Implement feature X"));
    expect(mockFabric.run).toHaveBeenCalledWith("plan", expect.stringContaining("understanding text"));
  });

  it("returns the plan string from fabric output", async () => {
    const result = await runPlan(config, mockFabric as any, mockSlack as any, "ts-1", "understanding");

    expect(result.plan).toBe("1. Parse input\n2. Transform\n3. Output");
  });

  it("posts plan to Slack thread", async () => {
    await runPlan(config, mockFabric as any, mockSlack as any, "ts-1", "understanding");

    expect(mockSlack.postToThread).toHaveBeenCalledWith(
      "C123",
      "ts-1",
      expect.stringContaining("Implementation Plan"),
    );
  });

  it("truncates long plans in Slack message", async () => {
    mockFabric.run.mockResolvedValue({
      exitCode: 0,
      stdout: "X".repeat(4000),
      stderr: "",
    });

    await runPlan(config, mockFabric as any, mockSlack as any, "ts-1", "understanding");

    const slackMsg = mockSlack.postToThread.mock.calls[0][2];
    expect(slackMsg).toContain("...(truncated)");
  });

  it("throws when fabric fails", async () => {
    mockFabric.run.mockResolvedValue({ exitCode: 1, stdout: "", stderr: "pattern error" });

    await expect(
      runPlan(config, mockFabric as any, mockSlack as any, "ts-1", "understanding"),
    ).rejects.toThrow("Fabric plan pattern failed");
  });
});
