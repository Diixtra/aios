import { describe, it, expect, vi, beforeEach } from "vitest";
import type { TaskConfig, ToolPolicy } from "./types.js";

// Mock all dependencies before importing executor
vi.mock("./fabric.js");
vi.mock("./github.js");
vi.mock("./memory.js");
vi.mock("./slack.js");
vi.mock("./pipeline/research.js");
vi.mock("./pipeline/understand.js");
vi.mock("./pipeline/plan.js");
vi.mock("./pipeline/implement.js");
vi.mock("./pipeline/verify.js");
vi.mock("./pipeline/deliver.js");

import { executeTask } from "./executor.js";
import { FabricRunner } from "./fabric.js";
import { GitHubClient } from "./github.js";
import { MemoryClient } from "./memory.js";
import { SlackNotifier } from "./slack.js";
import { runResearch } from "./pipeline/research.js";
import { runUnderstand } from "./pipeline/understand.js";
import { runPlan } from "./pipeline/plan.js";
import { runImplement } from "./pipeline/implement.js";
import { runVerify } from "./pipeline/verify.js";
import { runDeliver } from "./pipeline/deliver.js";

const mockSlack = {
  postTaskStarted: vi.fn().mockResolvedValue("thread-ts-123"),
  postToThread: vi.fn().mockResolvedValue(undefined),
  postTaskCompleted: vi.fn().mockResolvedValue(undefined),
  postEscalation: vi.fn().mockResolvedValue(undefined),
};

vi.mocked(SlackNotifier).mockImplementation(() => mockSlack as any);
vi.mocked(FabricRunner).mockImplementation(() => ({}) as any);
vi.mocked(GitHubClient).mockImplementation(() => ({}) as any);
vi.mocked(MemoryClient).mockImplementation(() => ({}) as any);

const baseConfig: TaskConfig = {
  taskId: "task-1",
  taskType: "code",
  prompt: "Fix the bug",
  repo: "owner/repo",
  branch: "fix-branch",
  slackChannel: "C123",
  memoryUrl: "http://memory:8080",
  searchUrl: "http://search:8080",
  workspace: "/workspace",
};

const toolPolicy: ToolPolicy = {
  allowedCommands: ["git ", "npm ", "npx ", "node "],
  deniedCommands: ["rm -rf /", "sudo "],
  writablePaths: ["/workspace/**"],
  readablePaths: ["/workspace/**"],
};

describe("executeTask", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockSlack.postTaskStarted.mockResolvedValue("thread-ts-123");
  });

  describe("research tasks", () => {
    it("runs only the research stage for research tasks", async () => {
      const config = { ...baseConfig, taskType: "research" as const };
      vi.mocked(runResearch).mockResolvedValue({
        outputPath: "/workspace/output/research.md",
        summary: "Research results",
      });

      const result = await executeTask(config, toolPolicy);

      expect(result.success).toBe(true);
      expect(result.taskType).toBe("research");
      expect(result.outputPath).toBe("/workspace/output/research.md");
      expect(runResearch).toHaveBeenCalledTimes(1);
      expect(runUnderstand).not.toHaveBeenCalled();
      expect(runPlan).not.toHaveBeenCalled();
      expect(runImplement).not.toHaveBeenCalled();
      expect(runVerify).not.toHaveBeenCalled();
      expect(runDeliver).not.toHaveBeenCalled();
    });
  });

  describe("code tasks", () => {
    it("runs stages in order: understand -> plan -> implement -> verify -> deliver", async () => {
      const callOrder: string[] = [];

      vi.mocked(runUnderstand).mockImplementation(async () => {
        callOrder.push("understand");
        return { issueBody: "issue", context: "ctx", enrichedUnderstanding: "enriched" };
      });
      vi.mocked(runPlan).mockImplementation(async () => {
        callOrder.push("plan");
        return { plan: "the plan" };
      });
      vi.mocked(runImplement).mockImplementation(async () => {
        callOrder.push("implement");
        return { success: true, summary: "done" };
      });
      vi.mocked(runVerify).mockImplementation(async () => {
        callOrder.push("verify");
        return { passed: true, failureReasons: [] };
      });
      vi.mocked(runDeliver).mockImplementation(async () => {
        callOrder.push("deliver");
        return { prUrl: "https://github.com/owner/repo/pull/1" };
      });

      const result = await executeTask(baseConfig, toolPolicy);

      expect(result.success).toBe(true);
      expect(result.prUrl).toBe("https://github.com/owner/repo/pull/1");
      expect(callOrder).toEqual(["understand", "plan", "implement", "verify", "deliver"]);
    });

    it("retries verify up to 3 times on failure", async () => {
      vi.mocked(runUnderstand).mockResolvedValue({
        issueBody: "issue",
        context: "ctx",
        enrichedUnderstanding: "enriched",
      });
      vi.mocked(runPlan).mockResolvedValue({ plan: "the plan" });
      vi.mocked(runImplement).mockResolvedValue({ success: true, summary: "done" });
      vi.mocked(runVerify).mockResolvedValue({
        passed: false,
        failureReasons: ["Tests failed"],
      });

      const result = await executeTask(baseConfig, toolPolicy);

      expect(result.success).toBe(false);
      expect(result.error).toContain("Verification failed after 3 attempts");
      expect(result.verifyAttempts).toBe(3);
      expect(runVerify).toHaveBeenCalledTimes(3);
      // Re-implement called between retries (not after last failure)
      expect(runImplement).toHaveBeenCalledTimes(3); // 1 initial + 2 retries
      expect(mockSlack.postEscalation).toHaveBeenCalledTimes(1);
    });

    it("passes on second verify attempt after initial failure", async () => {
      vi.mocked(runUnderstand).mockResolvedValue({
        issueBody: "issue",
        context: "ctx",
        enrichedUnderstanding: "enriched",
      });
      vi.mocked(runPlan).mockResolvedValue({ plan: "the plan" });
      vi.mocked(runImplement).mockResolvedValue({ success: true, summary: "done" });
      vi.mocked(runVerify)
        .mockResolvedValueOnce({ passed: false, failureReasons: ["Tests failed"] })
        .mockResolvedValueOnce({ passed: true, failureReasons: [] });
      vi.mocked(runDeliver).mockResolvedValue({
        prUrl: "https://github.com/owner/repo/pull/1",
      });

      const result = await executeTask(baseConfig, toolPolicy);

      expect(result.success).toBe(true);
      expect(result.verifyAttempts).toBe(2);
      expect(runVerify).toHaveBeenCalledTimes(2);
      expect(runDeliver).toHaveBeenCalledTimes(1);
    });

    it("escalates to human when all verify attempts fail", async () => {
      vi.mocked(runUnderstand).mockResolvedValue({
        issueBody: "issue",
        context: "ctx",
        enrichedUnderstanding: "enriched",
      });
      vi.mocked(runPlan).mockResolvedValue({ plan: "the plan" });
      vi.mocked(runImplement).mockResolvedValue({ success: true, summary: "done" });
      vi.mocked(runVerify).mockResolvedValue({
        passed: false,
        failureReasons: ["Tests failed"],
      });

      const result = await executeTask(baseConfig, toolPolicy);

      expect(result.success).toBe(false);
      expect(mockSlack.postEscalation).toHaveBeenCalledWith(
        "thread-ts-123",
        "task-1",
        "Verification failed after 3 attempts",
      );
      expect(runDeliver).not.toHaveBeenCalled();
    });
  });

  describe("error handling", () => {
    it("catches errors and returns failure result", async () => {
      vi.mocked(runUnderstand).mockRejectedValue(new Error("GitHub API down"));

      const result = await executeTask(baseConfig, toolPolicy);

      expect(result.success).toBe(false);
      expect(result.error).toBe("GitHub API down");
    });

    it("handles non-Error throws", async () => {
      vi.mocked(runUnderstand).mockRejectedValue("string error");

      const result = await executeTask(baseConfig, toolPolicy);

      expect(result.success).toBe(false);
      expect(result.error).toBe("Unknown error");
    });
  });

  describe("sandbox integration", () => {
    it("passes sandbox to verify stage", async () => {
      vi.mocked(runUnderstand).mockResolvedValue({
        issueBody: "issue",
        context: "ctx",
        enrichedUnderstanding: "enriched",
      });
      vi.mocked(runPlan).mockResolvedValue({ plan: "the plan" });
      vi.mocked(runImplement).mockResolvedValue({ success: true, summary: "done" });
      vi.mocked(runVerify).mockResolvedValue({ passed: true, failureReasons: [] });
      vi.mocked(runDeliver).mockResolvedValue({
        prUrl: "https://github.com/owner/repo/pull/1",
      });

      await executeTask(baseConfig, toolPolicy);

      // Verify that sandbox is passed as the last argument to runVerify
      const verifyCall = vi.mocked(runVerify).mock.calls[0];
      expect(verifyCall).toHaveLength(6);
      // The 6th arg should be a Sandbox instance
      expect(verifyCall[5]).toBeDefined();
      expect(typeof verifyCall[5].validateCommand).toBe("function");
    });

    it("passes sandbox to implement stage", async () => {
      vi.mocked(runUnderstand).mockResolvedValue({
        issueBody: "issue",
        context: "ctx",
        enrichedUnderstanding: "enriched",
      });
      vi.mocked(runPlan).mockResolvedValue({ plan: "the plan" });
      vi.mocked(runImplement).mockResolvedValue({ success: true, summary: "done" });
      vi.mocked(runVerify).mockResolvedValue({ passed: true, failureReasons: [] });
      vi.mocked(runDeliver).mockResolvedValue({
        prUrl: "https://github.com/owner/repo/pull/1",
      });

      await executeTask(baseConfig, toolPolicy);

      // Verify that sandbox is passed as the last argument to runImplement
      const implCall = vi.mocked(runImplement).mock.calls[0];
      expect(implCall).toHaveLength(5);
      expect(implCall[4]).toBeDefined();
      expect(typeof implCall[4].validateCommand).toBe("function");
    });
  });
});
