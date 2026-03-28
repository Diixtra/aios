import { loadTaskConfig, loadToolPolicy } from "./config.js";
import { executeTask } from "./executor.js";

/**
 * AIOS Runtime entrypoint.
 * Loads configuration from environment, loads tool policy, and executes the task pipeline.
 */
async function main(): Promise<void> {
  console.log("AIOS Runtime starting...");

  // Load configuration
  const config = loadTaskConfig();
  console.log(
    `Task: ${config.taskId} (${config.taskType}) - Repo: ${config.repo}`,
  );

  // Load tool policy
  const toolPolicy = await loadToolPolicy();
  console.log(
    `Tool policy loaded: ${toolPolicy.allowedCommands.length} allowed, ${toolPolicy.deniedCommands.length} denied`,
  );

  // Execute task
  const result = await executeTask(config, toolPolicy);

  // Log result
  if (result.success) {
    console.log(`Task ${result.taskId} completed successfully in ${result.durationMs}ms`);
    if (result.prUrl) {
      console.log(`PR: ${result.prUrl}`);
    }
    if (result.outputPath) {
      console.log(`Output: ${result.outputPath}`);
    }
  } else {
    console.error(`Task ${result.taskId} failed: ${result.error}`);
    process.exitCode = 1;
  }
}

main().catch((error) => {
  console.error("Fatal error:", error);
  process.exitCode = 1;
});
