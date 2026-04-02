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
