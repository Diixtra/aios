import { describe, it, expect, vi, beforeEach } from "vitest";
import { runVerify } from "./verify.js";
import { Sandbox } from "../sandbox.js";
import type { ToolPolicy, TaskConfig } from "../types.js";

// Mock the sandbox module's sandboxedExec
vi.mock("../sandbox.js", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../sandbox.js")>();
  return {
    ...actual,
    sandboxedExec: vi.fn(),
  };
});

import { sandboxedExec } from "../sandbox.js";

const mockFabric = {
  run: vi.fn(),
};

const mockSlack = {
  postToThread: vi.fn().mockResolvedValue(undefined),
};

const config: TaskConfig = {
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
  allowedCommands: ["git ", "npm "],
  deniedCommands: ["rm -rf /"],
  writablePaths: ["/workspace/**"],
  readablePaths: ["/workspace/**"],
};

describe("runVerify", () => {
  let sandbox: Sandbox;

  beforeEach(() => {
    vi.clearAllMocks();
    sandbox = new Sandbox(toolPolicy);
  });

  it("passes when tests and review succeed", async () => {
    vi.mocked(sandboxedExec)
      .mockResolvedValueOnce({ exitCode: 0, stdout: "All tests passed", stderr: "" }) // npm test
      .mockResolvedValueOnce({ exitCode: 0, stdout: "diff output", stderr: "" }); // git diff

    mockFabric.run.mockResolvedValue({ exitCode: 0, stdout: "Review: LGTM", stderr: "" });

    const result = await runVerify(
      config,
      mockFabric as any,
      mockSlack as any,
      "thread-ts",
      1,
      sandbox,
    );

    expect(result.passed).toBe(true);
    expect(result.failureReasons).toHaveLength(0);
    expect(result.testResult?.exitCode).toBe(0);
  });

  it("fails when tests fail", async () => {
    vi.mocked(sandboxedExec)
      .mockResolvedValueOnce({ exitCode: 1, stdout: "", stderr: "FAIL" }) // npm test fails
      .mockResolvedValueOnce({ exitCode: 0, stdout: "", stderr: "" }); // git diff (empty)

    const result = await runVerify(
      config,
      mockFabric as any,
      mockSlack as any,
      "thread-ts",
      1,
      sandbox,
    );

    expect(result.passed).toBe(false);
    expect(result.failureReasons).toContain("Tests failed (exit code 1)");
  });

  it("fails when sandbox blocks the command", async () => {
    vi.mocked(sandboxedExec)
      .mockResolvedValueOnce({
        exitCode: 126,
        stdout: "",
        stderr: "Sandbox blocked command: Command does not match any allowed prefix",
      })
      .mockResolvedValueOnce({ exitCode: 0, stdout: "", stderr: "" });

    const result = await runVerify(
      config,
      mockFabric as any,
      mockSlack as any,
      "thread-ts",
      1,
      sandbox,
    );

    expect(result.passed).toBe(false);
    expect(result.failureReasons[0]).toContain("Tests failed (exit code 126)");
  });

  it("includes fabric review failure in reasons", async () => {
    vi.mocked(sandboxedExec)
      .mockResolvedValueOnce({ exitCode: 0, stdout: "Tests passed", stderr: "" })
      .mockResolvedValueOnce({ exitCode: 0, stdout: "diff content here", stderr: "" });

    mockFabric.run.mockResolvedValue({ exitCode: 1, stdout: "", stderr: "review failed" });

    const result = await runVerify(
      config,
      mockFabric as any,
      mockSlack as any,
      "thread-ts",
      1,
      sandbox,
    );

    expect(result.passed).toBe(false);
    expect(result.failureReasons).toContain("Fabric review pattern failed");
  });

  it("skips fabric review when diff is empty", async () => {
    vi.mocked(sandboxedExec)
      .mockResolvedValueOnce({ exitCode: 0, stdout: "Tests passed", stderr: "" })
      .mockResolvedValueOnce({ exitCode: 0, stdout: "   ", stderr: "" }); // empty diff

    const result = await runVerify(
      config,
      mockFabric as any,
      mockSlack as any,
      "thread-ts",
      1,
      sandbox,
    );

    expect(result.passed).toBe(true);
    expect(mockFabric.run).not.toHaveBeenCalled();
    expect(result.reviewOutput).toBeUndefined();
  });

  it("uses sandboxedExec instead of raw execFile", async () => {
    vi.mocked(sandboxedExec)
      .mockResolvedValueOnce({ exitCode: 0, stdout: "ok", stderr: "" })
      .mockResolvedValueOnce({ exitCode: 0, stdout: "", stderr: "" });

    await runVerify(
      config,
      mockFabric as any,
      mockSlack as any,
      "thread-ts",
      1,
      sandbox,
    );

    expect(sandboxedExec).toHaveBeenCalledWith(sandbox, "npm", ["test"], "/workspace");
    expect(sandboxedExec).toHaveBeenCalledWith(sandbox, "git", ["diff", "--cached"], "/workspace");
  });

  it("posts slack notifications for pass and fail", async () => {
    vi.mocked(sandboxedExec)
      .mockResolvedValueOnce({ exitCode: 0, stdout: "ok", stderr: "" })
      .mockResolvedValueOnce({ exitCode: 0, stdout: "", stderr: "" });

    await runVerify(
      config,
      mockFabric as any,
      mockSlack as any,
      "thread-ts",
      2,
      sandbox,
    );

    // Should post "Running verification" and "passed" messages
    expect(mockSlack.postToThread).toHaveBeenCalledTimes(2);
    expect(mockSlack.postToThread).toHaveBeenCalledWith(
      "C123",
      "thread-ts",
      expect.stringContaining("attempt 2"),
    );
  });
});
