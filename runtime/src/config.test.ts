import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { loadTaskConfig, loadToolPolicy } from "./config.js";

// Mock node:fs/promises
vi.mock("node:fs/promises", () => ({
  readFile: vi.fn(),
}));

const REQUIRED_ENV = {
  AIOS_TASK_ID: "task-123",
  AIOS_TASK_TYPE: "code",
  AIOS_PROMPT: "Fix the bug",
  AIOS_REPO: "owner/repo",
  AIOS_BRANCH: "fix-branch",
  AIOS_SLACK_CHANNEL: "C123",
  AIOS_MEMORY_URL: "http://memory:8080",
  AIOS_SEARCH_URL: "http://search:8080",
};

describe("loadTaskConfig", () => {
  let originalEnv: NodeJS.ProcessEnv;

  beforeEach(() => {
    originalEnv = { ...process.env };
    // Set all required vars
    Object.assign(process.env, REQUIRED_ENV);
  });

  afterEach(() => {
    process.env = originalEnv;
  });

  it("returns correct TaskConfig with all required vars", () => {
    const config = loadTaskConfig();

    expect(config.taskId).toBe("task-123");
    expect(config.taskType).toBe("code");
    expect(config.prompt).toBe("Fix the bug");
    expect(config.repo).toBe("owner/repo");
    expect(config.branch).toBe("fix-branch");
    expect(config.slackChannel).toBe("C123");
    expect(config.memoryUrl).toBe("http://memory:8080");
    expect(config.searchUrl).toBe("http://search:8080");
    expect(config.workspace).toBe("/workspace");
  });

  it("parses optional issue number", () => {
    process.env.AIOS_ISSUE_NUMBER = "42";
    const config = loadTaskConfig();
    expect(config.issueNumber).toBe(42);
  });

  it("leaves issueNumber undefined when not set", () => {
    delete process.env.AIOS_ISSUE_NUMBER;
    const config = loadTaskConfig();
    expect(config.issueNumber).toBeUndefined();
  });

  it("treats issueNumber 0 as undefined", () => {
    process.env.AIOS_ISSUE_NUMBER = "0";
    const config = loadTaskConfig();
    expect(config.issueNumber).toBeUndefined();
  });

  it("uses optional slack thread ts", () => {
    process.env.AIOS_SLACK_THREAD_TS = "1234.5678";
    const config = loadTaskConfig();
    expect(config.slackThreadTs).toBe("1234.5678");
  });

  it("uses custom workspace when set", () => {
    process.env.AIOS_WORKSPACE = "/custom/workspace";
    const config = loadTaskConfig();
    expect(config.workspace).toBe("/custom/workspace");
  });

  it("throws when AIOS_TASK_ID is missing", () => {
    delete process.env.AIOS_TASK_ID;
    expect(() => loadTaskConfig()).toThrow("Missing required environment variable: AIOS_TASK_ID");
  });

  it("throws when AIOS_PROMPT is missing", () => {
    delete process.env.AIOS_PROMPT;
    expect(() => loadTaskConfig()).toThrow("Missing required environment variable: AIOS_PROMPT");
  });

  it("throws when AIOS_REPO is missing", () => {
    delete process.env.AIOS_REPO;
    expect(() => loadTaskConfig()).toThrow("Missing required environment variable: AIOS_REPO");
  });

  it("loads model and maxTokens from env", () => {
    process.env.AIOS_CLAUDE_MODEL = "claude-haiku-4-5-20251001";
    process.env.AIOS_CLAUDE_MAX_TOKENS = "32768";

    const config = loadTaskConfig();
    expect(config.model).toBe("claude-haiku-4-5-20251001");
    expect(config.maxTokens).toBe(32768);
  });

  it("falls back to default maxTokens for non-numeric value", () => {
    process.env.AIOS_CLAUDE_MAX_TOKENS = "abc";

    const config = loadTaskConfig();
    expect(config.maxTokens).toBe(16384);
  });

  it("falls back to default maxTokens for zero or negative value", () => {
    process.env.AIOS_CLAUDE_MAX_TOKENS = "0";
    expect(loadTaskConfig().maxTokens).toBe(16384);

    process.env.AIOS_CLAUDE_MAX_TOKENS = "-100";
    expect(loadTaskConfig().maxTokens).toBe(16384);
  });

  it("uses defaults for model and maxTokens when not set", () => {
    delete process.env.AIOS_CLAUDE_MODEL;
    delete process.env.AIOS_CLAUDE_MAX_TOKENS;

    const config = loadTaskConfig();
    expect(config.model).toBe("claude-sonnet-4-6");
    expect(config.maxTokens).toBe(16384);
  });
});

describe("loadToolPolicy", () => {
  it("returns parsed policy from valid JSON file", async () => {
    const { readFile } = await import("node:fs/promises");
    const mockReadFile = vi.mocked(readFile);

    const policyJson = JSON.stringify({
      allowedCommands: ["git ", "npm "],
      deniedCommands: ["rm -rf"],
      writablePaths: ["/workspace/**"],
      readablePaths: ["/workspace/**", "/etc/aios/**"],
    });
    mockReadFile.mockResolvedValue(policyJson);

    const policy = await loadToolPolicy("/etc/aios/toolpolicy/policy.json");

    expect(policy.allowedCommands).toEqual(["git ", "npm "]);
    expect(policy.deniedCommands).toEqual(["rm -rf"]);
    expect(policy.writablePaths).toEqual(["/workspace/**"]);
    expect(policy.readablePaths).toEqual(["/workspace/**", "/etc/aios/**"]);
  });

  it("returns deny-all fallback when file is missing", async () => {
    const { readFile } = await import("node:fs/promises");
    const mockReadFile = vi.mocked(readFile);
    mockReadFile.mockRejectedValue(new Error("ENOENT"));

    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const policy = await loadToolPolicy("/nonexistent/path.json");

    expect(policy.allowedCommands).toEqual([]);
    expect(policy.deniedCommands).toEqual([]);
    expect(policy.writablePaths).toEqual([]);
    expect(policy.readablePaths).toEqual([]);
    expect(consoleSpy).toHaveBeenCalledWith(expect.stringContaining("FATAL"));
    consoleSpy.mockRestore();
  });

  it("fills missing fields with empty arrays", async () => {
    const { readFile } = await import("node:fs/promises");
    const mockReadFile = vi.mocked(readFile);
    mockReadFile.mockResolvedValue(JSON.stringify({ allowedCommands: ["git "] }));

    const policy = await loadToolPolicy("/some/path.json");

    expect(policy.allowedCommands).toEqual(["git "]);
    expect(policy.deniedCommands).toEqual([]);
    expect(policy.writablePaths).toEqual([]);
    expect(policy.readablePaths).toEqual([]);
  });
});
