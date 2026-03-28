import type { TaskConfig } from "../types.js";
import type { SlackNotifier } from "../slack.js";
import type { Sandbox } from "../sandbox.js";

export interface ImplementResult {
  /** Whether implementation completed without errors */
  success: boolean;
  /** Summary of changes made */
  summary: string;
}

/**
 * Implement stage: executes the implementation plan using Claude Agent SDK.
 *
 * TODO: Integrate Claude Agent SDK for autonomous code generation.
 * Currently a stub that will be wired to the SDK in a future task.
 *
 * The implementation stage should:
 * 1. Take the plan from the plan stage
 * 2. Use Claude Agent SDK to generate code changes
 * 3. Apply changes to the workspace
 * 4. Stage and commit changes
 */
export async function runImplement(
  config: TaskConfig,
  slack: SlackNotifier,
  threadTs: string,
  plan: string,
  _sandbox: Sandbox,
): Promise<ImplementResult> {
  await slack.postToThread(
    config.slackChannel,
    threadTs,
    `:hammer_and_wrench: Starting implementation...`,
  );

  // TODO: Wire up Claude Agent SDK
  // const agent = new ClaudeAgent({
  //   workspace: config.workspace,
  //   plan,
  //   toolPolicy: sandbox,
  // });
  // const result = await agent.execute();

  await slack.postToThread(
    config.slackChannel,
    threadTs,
    `:white_check_mark: Implementation stage complete.`,
  );

  return {
    success: true,
    summary: "Implementation completed (stub - Claude Agent SDK pending)",
  };
}
