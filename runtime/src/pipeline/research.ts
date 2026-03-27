import { writeFile, mkdir } from "node:fs/promises";
import { join } from "node:path";
import type { TaskConfig } from "../types.js";
import type { MemoryClient } from "../memory.js";
import type { FabricRunner } from "../fabric.js";
import type { SlackNotifier } from "../slack.js";

export interface ResearchResult {
  outputPath: string;
  summary: string;
}

/**
 * Research stage: searches memory/vault, enriches with fabric, writes output.
 */
export async function runResearch(
  config: TaskConfig,
  memory: MemoryClient,
  fabric: FabricRunner,
  slack: SlackNotifier,
  threadTs: string,
): Promise<ResearchResult> {
  const outputDir = join(config.workspace, "output");
  await mkdir(outputDir, { recursive: true });

  // Search memory for relevant context
  const memoryResults = await memory.searchMemory(config.prompt, 10);
  const vaultResults = await memory.semanticSearch(config.prompt, 10);

  // Combine context
  const context = [
    "## Memory Results",
    ...memoryResults.map((r) => `- ${r.content}`),
    "",
    "## Vault Results",
    ...vaultResults.map((r) => `- ${r.content}`),
    "",
    "## Task Prompt",
    config.prompt,
  ].join("\n");

  // Enrich with fabric research pattern
  const enriched = await fabric.run("research", context);

  if (enriched.exitCode !== 0) {
    throw new Error(`Fabric research pattern failed: ${enriched.stderr}`);
  }

  // Write output
  const outputPath = join(outputDir, "research.md");
  await writeFile(outputPath, enriched.stdout, "utf-8");

  // Store in memory for future reference
  await memory.storeMemory(`research:${config.taskId}`, enriched.stdout, {
    taskId: config.taskId,
    type: "research",
  });

  // Notify Slack
  await slack.postToThread(
    config.slackChannel,
    threadTs,
    `:mag: Research complete. Output written to ${outputPath}`,
  );

  return { outputPath, summary: enriched.stdout.slice(0, 500) };
}
