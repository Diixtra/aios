import { describe, it, expect, vi, beforeEach } from "vitest";
import { runUnderstand } from "./understand.js";
import type { TaskConfig } from "../types.js";

const config: TaskConfig = {
  taskId: "task-1",
  taskType: "code",
  prompt: "Fix authentication bug",
  repo: "owner/repo",
  issueNumber: 42,
  branch: "fix-auth",
  slackChannel: "C123",
  memoryUrl: "http://memory:8080",
  searchUrl: "http://search:8080",
  workspace: "/workspace",
};

const mockGithub = {
  getIssueBody: vi.fn(),
};

const mockMemory = {
  searchMemory: vi.fn(),
  semanticSearch: vi.fn(),
};

const mockFabric = {
  run: vi.fn(),
};

const mockSlack = {
  postToThread: vi.fn().mockResolvedValue(undefined),
};

describe("runUnderstand", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGithub.getIssueBody.mockResolvedValue("Auth tokens expire too quickly");
    mockMemory.searchMemory.mockResolvedValue([
      { content: "Token refresh was added in v2.1", score: 0.85 },
    ]);
    mockMemory.semanticSearch.mockResolvedValue([
      { file_path: "docs/auth.md", title: "Auth Architecture", snippet: "Auth architecture doc", score: 0.9 },
    ]);
    mockFabric.run.mockResolvedValue({
      exitCode: 0,
      stdout: "The issue is about JWT token expiry handling.",
      stderr: "",
    });
  });

  it("fetches issue body from GitHub when issueNumber is set", async () => {
    await runUnderstand(
      config, mockGithub as any, mockMemory as any, mockFabric as any, mockSlack as any, "ts-1",
    );

    expect(mockGithub.getIssueBody).toHaveBeenCalledWith(42);
  });

  it("uses prompt as issue body when no issueNumber", async () => {
    const configNoIssue = { ...config, issueNumber: undefined };

    const result = await runUnderstand(
      configNoIssue, mockGithub as any, mockMemory as any, mockFabric as any, mockSlack as any, "ts-1",
    );

    expect(mockGithub.getIssueBody).not.toHaveBeenCalled();
    expect(result.issueBody).toBe("Fix authentication bug");
  });

  it("searches memory and vault with issue body", async () => {
    await runUnderstand(
      config, mockGithub as any, mockMemory as any, mockFabric as any, mockSlack as any, "ts-1",
    );

    expect(mockMemory.searchMemory).toHaveBeenCalledWith("Auth tokens expire too quickly", 10);
    expect(mockMemory.semanticSearch).toHaveBeenCalledWith("Auth tokens expire too quickly", 10);
  });

  it("calls fabric with understand pattern and combined context", async () => {
    await runUnderstand(
      config, mockGithub as any, mockMemory as any, mockFabric as any, mockSlack as any, "ts-1",
    );

    expect(mockFabric.run).toHaveBeenCalledWith("understand", expect.stringContaining("Auth tokens expire too quickly"));
    expect(mockFabric.run).toHaveBeenCalledWith("understand", expect.stringContaining("Token refresh was added in v2.1"));
    expect(mockFabric.run).toHaveBeenCalledWith("understand", expect.stringContaining("Auth architecture doc"));
  });

  it("returns enriched understanding from fabric", async () => {
    const result = await runUnderstand(
      config, mockGithub as any, mockMemory as any, mockFabric as any, mockSlack as any, "ts-1",
    );

    expect(result.enrichedUnderstanding).toBe("The issue is about JWT token expiry handling.");
    expect(result.issueBody).toBe("Auth tokens expire too quickly");
    expect(result.context).toContain("Token refresh was added in v2.1");
  });

  it("posts to Slack with result counts", async () => {
    await runUnderstand(
      config, mockGithub as any, mockMemory as any, mockFabric as any, mockSlack as any, "ts-1",
    );

    expect(mockSlack.postToThread).toHaveBeenCalledWith(
      "C123",
      "ts-1",
      expect.stringContaining("1 memory results"),
    );
    expect(mockSlack.postToThread).toHaveBeenCalledWith(
      "C123",
      "ts-1",
      expect.stringContaining("1 vault docs"),
    );
  });

  it("throws when fabric fails", async () => {
    mockFabric.run.mockResolvedValue({ exitCode: 1, stdout: "", stderr: "pattern error" });

    await expect(
      runUnderstand(config, mockGithub as any, mockMemory as any, mockFabric as any, mockSlack as any, "ts-1"),
    ).rejects.toThrow("Fabric understand pattern failed");
  });
});
