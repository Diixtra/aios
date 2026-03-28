import type { TaskConfig } from "../types.js";
import type { FabricRunner } from "../fabric.js";
import type { SlackNotifier } from "../slack.js";

export interface PlanResult {
  plan: string;
}

/**
 * Plan stage: generates an implementation plan and posts it to Slack.
 */
export async function runPlan(
  config: TaskConfig,
  fabric: FabricRunner,
  slack: SlackNotifier,
  threadTs: string,
  understanding: string,
): Promise<PlanResult> {
  const planInput = [
    "## Task",
    config.prompt,
    "",
    "## Understanding",
    understanding,
  ].join("\n");

  const result = await fabric.run("plan", planInput);

  if (result.exitCode !== 0) {
    throw new Error(`Fabric plan pattern failed: ${result.stderr}`);
  }

  const plan = result.stdout;

  // Post plan to Slack for visibility
  await slack.postToThread(
    config.slackChannel,
    threadTs,
    `:clipboard: *Implementation Plan:*\n${plan.slice(0, 3000)}${plan.length > 3000 ? "\n...(truncated)" : ""}`,
  );

  return { plan };
}
