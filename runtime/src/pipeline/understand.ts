import type { TaskConfig } from "../types.js";
import type { MemoryClient } from "../memory.js";
import type { FabricRunner } from "../fabric.js";
import type { GitHubClient } from "../github.js";
import type { SlackNotifier } from "../slack.js";

export interface UnderstandResult {
  issueBody: string;
  context: string;
  enrichedUnderstanding: string;
}

/**
 * Understand stage: fetches issue, searches memory/vault, enriches with fabric.
 */
export async function runUnderstand(
  config: TaskConfig,
  github: GitHubClient,
  memory: MemoryClient,
  fabric: FabricRunner,
  slack: SlackNotifier,
  threadTs: string,
): Promise<UnderstandResult> {
  // Fetch issue body if available
  let issueBody = config.prompt;
  if (config.issueNumber) {
    issueBody = await github.getIssueBody(config.issueNumber);
  }

  // Search memory and vault for relevant context
  const memoryResults = await memory.searchMemory(issueBody, 10);
  const vaultResults = await memory.semanticSearch(issueBody, 10);

  const context = [
    "## Issue",
    issueBody,
    "",
    "## Relevant Memory",
    ...memoryResults.map((r) => `- [${r.score.toFixed(2)}] ${r.content}`),
    "",
    "## Relevant Vault Docs",
    ...vaultResults.map((r) => `- [${r.score.toFixed(2)}] ${r.snippet}`),
  ].join("\n");

  // Enrich understanding with fabric
  const enriched = await fabric.run("understand", context);

  if (enriched.exitCode !== 0) {
    throw new Error(`Fabric understand pattern failed: ${enriched.stderr}`);
  }

  await slack.postToThread(
    config.slackChannel,
    threadTs,
    `:brain: Understanding complete. Found ${memoryResults.length} memory results and ${vaultResults.length} vault docs.`,
  );

  return {
    issueBody,
    context,
    enrichedUnderstanding: enriched.stdout,
  };
}
