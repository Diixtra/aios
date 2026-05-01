import { describe, it, expect, vi, beforeEach } from "vitest";
import { runResearch } from "./research.js";
import type { TaskConfig } from "../types.js";

// Mock node:fs/promises
vi.mock("node:fs/promises", () => ({
  writeFile: vi.fn().mockResolvedValue(undefined),
  mkdir: vi.fn().mockResolvedValue(undefined),
}));

const config: TaskConfig = {
  taskId: "task-1",
  taskType: "research",
  prompt: "Research distributed caching",
  repo: "owner/repo",
  branch: "research-branch",
  slackChannel: "C123",
  memoryUrl: "http://memory:8080",
  searchUrl: "http://search:8080",
  workspace: "/workspace",
  model: "claude-sonnet-4-6",
  maxTokens: 16384,
};

const mockMemory = {
  searchMemory: vi.fn(),
  semanticSearch: vi.fn(),
  storeMemory: vi.fn().mockResolvedValue(undefined),
};

const mockFabric = {
  run: vi.fn(),
};

const mockSlack = {
  postToThread: vi.fn().mockResolvedValue(undefined),
};

describe("runResearch", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockMemory.searchMemory.mockResolvedValue([
      { content: "Redis is fast", score: 0.9 },
    ]);
    mockMemory.semanticSearch.mockResolvedValue([
      { file_path: "docs/caching.md", title: "Caching Comparison", snippet: "Memcached comparison doc", score: 0.8 },
    ]);
    mockFabric.run.mockResolvedValue({
      exitCode: 0,
      stdout: "# Research findings\nDistributed caching overview...",
      stderr: "",
    });
  });

  it("searches memory and vault for context", async () => {
    await runResearch(config, mockMemory as any, mockFabric as any, mockSlack as any, "ts-1");

    expect(mockMemory.searchMemory).toHaveBeenCalledWith("Research distributed caching", 10);
    expect(mockMemory.semanticSearch).toHaveBeenCalledWith("Research distributed caching", 10);
  });

  it("calls fabric with research pattern and combined context", async () => {
    await runResearch(config, mockMemory as any, mockFabric as any, mockSlack as any, "ts-1");

    expect(mockFabric.run).toHaveBeenCalledWith("research", expect.stringContaining("Redis is fast"));
    expect(mockFabric.run).toHaveBeenCalledWith("research", expect.stringContaining("Memcached comparison doc"));
  });

  it("writes output file and returns path", async () => {
    const { writeFile } = await import("node:fs/promises");

    const result = await runResearch(config, mockMemory as any, mockFabric as any, mockSlack as any, "ts-1");

    expect(result.outputPath).toContain("research.md");
    expect(writeFile).toHaveBeenCalledWith(
      expect.stringContaining("research.md"),
      expect.stringContaining("Research findings"),
      "utf-8",
    );
  });

  it("stores research in memory", async () => {
    await runResearch(config, mockMemory as any, mockFabric as any, mockSlack as any, "ts-1");

    expect(mockMemory.storeMemory).toHaveBeenCalledWith(
      "research:task-1",
      expect.any(String),
      expect.objectContaining({ taskId: "task-1", type: "research" }),
    );
  });

  it("notifies Slack", async () => {
    await runResearch(config, mockMemory as any, mockFabric as any, mockSlack as any, "ts-1");

    expect(mockSlack.postToThread).toHaveBeenCalledWith(
      "C123",
      "ts-1",
      expect.stringContaining("Research complete"),
    );
  });

  it("throws when fabric fails", async () => {
    mockFabric.run.mockResolvedValue({ exitCode: 1, stdout: "", stderr: "pattern not found" });

    await expect(
      runResearch(config, mockMemory as any, mockFabric as any, mockSlack as any, "ts-1"),
    ).rejects.toThrow("Fabric research pattern failed");
  });

  it("returns a summary truncated to 500 chars", async () => {
    mockFabric.run.mockResolvedValue({
      exitCode: 0,
      stdout: "A".repeat(600),
      stderr: "",
    });

    const result = await runResearch(config, mockMemory as any, mockFabric as any, mockSlack as any, "ts-1");

    expect(result.summary.length).toBe(500);
  });
});
