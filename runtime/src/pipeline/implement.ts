import Anthropic from "@anthropic-ai/sdk";
import type { TaskConfig } from "../types.js";
import type { SlackNotifier } from "../slack.js";
import { Sandbox, sandboxedExec } from "../sandbox.js";
import {
  buildToolDefinitions,
  executeTool,
  type ToolCallBlock,
} from "../agent-tools.js";

export interface ImplementResult {
  /** Whether implementation completed without errors */
  success: boolean;
  /** Summary of changes made */
  summary: string;
}

const MAX_TURNS = 50;
const SLACK_THROTTLE_MS = 10_000;

/**
 * Implement stage: executes the implementation plan using Claude Agent SDK.
 *
 * Runs a tool-use agentic loop where Claude reads code, writes changes,
 * and runs commands — all sandboxed via the ToolPolicy. Stages and commits
 * changes on success.
 */
export async function runImplement(
  config: TaskConfig,
  slack: SlackNotifier,
  threadTs: string,
  plan: string,
  sandbox: Sandbox,
): Promise<ImplementResult> {
  await slack.postToThread(
    config.slackChannel,
    threadTs,
    `:hammer_and_wrench: Starting implementation...`,
  );

  const client = new Anthropic();
  const systemPrompt = buildSystemPrompt(config);
  const tools = buildToolDefinitions();
  const messages: Anthropic.MessageParam[] = [
    { role: "user", content: plan },
  ];

  try {
    let response = await client.messages.create({
      model: config.model,
      max_tokens: config.maxTokens,
      system: systemPrompt,
      messages,
      tools,
    });

    let turns = 0;
    let lastSlackPost = 0;

    while (response.stop_reason === "tool_use" && turns < MAX_TURNS) {
      turns++;
      const toolBlocks = response.content.filter(
        (b): b is Anthropic.ToolUseBlock => b.type === "tool_use",
      );

      const toolResults: Anthropic.ToolResultBlockParam[] = [];

      for (const block of toolBlocks) {
        // Throttled Slack progress
        const now = Date.now();
        if (now - lastSlackPost > SLACK_THROTTLE_MS) {
          const label =
            block.name === "shell"
              ? `:terminal: Running: ${(block.input as any).command}`
              : block.name === "write_file"
                ? `:pencil2: Writing: ${(block.input as any).path}`
                : `:mag: Reading: ${(block.input as any).path}`;
          await slack.postToThread(config.slackChannel, threadTs, label);
          lastSlackPost = now;
        }

        const result = await executeTool(
          block as ToolCallBlock,
          sandbox,
          config.workspace,
        );

        toolResults.push({
          type: "tool_result",
          tool_use_id: block.id,
          content: result.content,
          is_error: result.is_error,
        });
      }

      messages.push({ role: "assistant", content: response.content });
      messages.push({ role: "user", content: toolResults });

      response = await client.messages.create({
        model: config.model,
        max_tokens: config.maxTokens,
        system: systemPrompt,
        messages,
        tools,
      });
    }

    // Check if loop exited because of turn limit while agent still wanted tools
    if (response.stop_reason === "tool_use") {
      await slack.postToThread(
        config.slackChannel,
        threadTs,
        `:warning: Implementation hit turn limit (${MAX_TURNS})`,
      );
      return {
        success: false,
        summary: `Implementation exceeded turn limit (${MAX_TURNS})`,
      };
    }

    // Stage any unstaged changes and commit
    const commitResult = await stageAndCommit(sandbox, config);
    if (!commitResult.committed && commitResult.error) {
      await slack.postToThread(
        config.slackChannel,
        threadTs,
        `:x: Git commit failed: ${commitResult.error}`,
      );
      return { success: false, summary: `Git commit failed: ${commitResult.error}` };
    }

    const summary = extractSummary(response);

    await slack.postToThread(
      config.slackChannel,
      threadTs,
      `:white_check_mark: Implementation complete.`,
    );

    return { success: true, summary };
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    await slack.postToThread(
      config.slackChannel,
      threadTs,
      `:x: Implementation failed: ${message}`,
    );
    return { success: false, summary: message };
  }
}

interface CommitOutcome {
  committed: boolean;
  error?: string;
}

async function stageAndCommit(
  sandbox: Sandbox,
  config: TaskConfig,
): Promise<CommitOutcome> {
  const addResult = await sandboxedExec(sandbox, "git", ["add", "-A"], config.workspace);
  if (addResult.exitCode !== 0) {
    return { committed: false, error: addResult.stderr || `git add failed (exit ${addResult.exitCode})` };
  }

  // Check if there's anything to commit (avoids empty commits)
  const diffResult = await sandboxedExec(sandbox, "git", ["diff", "--cached", "--quiet"], config.workspace);
  if (diffResult.exitCode === 0) {
    // No staged changes — agent may have already committed, or made no file changes
    return { committed: false };
  }

  // Use a safe commit message that avoids metacharacters from taskId
  const safeTaskId = config.taskId.replace(/[^a-zA-Z0-9_\-. ]/g, "_");
  const commitResult = await sandboxedExec(
    sandbox,
    "git",
    ["commit", "-m", `aios: implement ${safeTaskId}`],
    config.workspace,
  );
  if (commitResult.exitCode !== 0) {
    return { committed: false, error: commitResult.stderr || `git commit failed (exit ${commitResult.exitCode})` };
  }

  return { committed: true };
}

function buildSystemPrompt(config: TaskConfig): string {
  return `You are a coding agent. Implement the plan provided by the user.

Workspace: ${config.workspace}
Repository: ${config.repo}

Rules:
- Only modify files within ${config.workspace}
- Run tests after making changes to verify correctness
- Follow existing code patterns and conventions
- Make small, focused changes
- If a command is blocked by the sandbox, find an alternative approach
- Do NOT run git add or git commit — staging and committing is handled automatically after you finish

You have three tools: shell (run commands), read_file, and write_file.`;
}

function extractSummary(response: Anthropic.Message): string {
  const textBlocks = response.content.filter(
    (b): b is Anthropic.TextBlock => b.type === "text",
  );
  if (textBlocks.length === 0) {
    return "Implementation completed (no summary provided)";
  }
  return textBlocks.map((b) => b.text).join("\n");
}
