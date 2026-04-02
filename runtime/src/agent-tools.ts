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

  const output = [
    `Exit code: ${result.exitCode}`,
    result.stdout ? `stdout:\n${result.stdout}` : "",
    result.stderr ? `stderr:\n${result.stderr}` : "",
  ]
    .filter(Boolean)
    .join("\n");

  return { content: output, is_error: result.exitCode !== 0 ? true : undefined };
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
