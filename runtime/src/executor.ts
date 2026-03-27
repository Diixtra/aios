import type { TaskConfig, ToolPolicy, PipelineResult } from "./types.js";
import { Sandbox } from "./sandbox.js";
import { FabricRunner } from "./fabric.js";
import { GitHubClient } from "./github.js";
import { MemoryClient } from "./memory.js";
import { SlackNotifier } from "./slack.js";
import { runResearch } from "./pipeline/research.js";
import { runUnderstand } from "./pipeline/understand.js";
import { runPlan } from "./pipeline/plan.js";
import { runImplement } from "./pipeline/implement.js";
import { runVerify } from "./pipeline/verify.js";
import { runDeliver } from "./pipeline/deliver.js";

/** Maximum number of verify-fix-verify retry attempts */
const MAX_VERIFY_ATTEMPTS = 3;

/**
 * Execute a task through the full pipeline.
 * - Research tasks: research stage only
 * - Code tasks: understand -> plan -> implement -> verify (with retries) -> deliver
 */
export async function executeTask(
  config: TaskConfig,
  toolPolicy: ToolPolicy,
): Promise<PipelineResult> {
  const startTime = Date.now();
  const sandbox = new Sandbox(toolPolicy);
  const fabric = new FabricRunner();
  const github = new GitHubClient(config.repo);
  const memory = new MemoryClient(config.memoryUrl, config.searchUrl);
  const slack = new SlackNotifier(config.slackChannel);

  try {
    // Start Slack thread
    const threadTs =
      config.slackThreadTs ??
      (await slack.postTaskStarted(config.taskId, config.repo, config.prompt));

    if (config.taskType === "research") {
      const result = await runResearch(config, memory, fabric, slack, threadTs);

      return {
        success: true,
        taskId: config.taskId,
        taskType: "research",
        outputPath: result.outputPath,
        durationMs: Date.now() - startTime,
      };
    }

    // Code task pipeline: understand -> plan -> implement -> verify -> deliver
    const understanding = await runUnderstand(
      config,
      github,
      memory,
      fabric,
      slack,
      threadTs,
    );

    const planResult = await runPlan(
      config,
      fabric,
      slack,
      threadTs,
      understanding.enrichedUnderstanding,
    );

    let implementResult = await runImplement(
      config,
      slack,
      threadTs,
      planResult.plan,
      sandbox,
    );

    // Verify with retry loop
    let verifyAttempts = 0;
    let verified = false;

    for (let attempt = 1; attempt <= MAX_VERIFY_ATTEMPTS; attempt++) {
      verifyAttempts = attempt;
      const verifyResult = await runVerify(
        config,
        fabric,
        slack,
        threadTs,
        attempt,
        sandbox,
      );

      if (verifyResult.passed) {
        verified = true;
        break;
      }

      if (attempt < MAX_VERIFY_ATTEMPTS) {
        // Re-implement with feedback from verification
        const feedback = verifyResult.failureReasons.join("\n");
        implementResult = await runImplement(
          config,
          slack,
          threadTs,
          `${planResult.plan}\n\n## Verification Feedback (attempt ${attempt})\n${feedback}`,
          sandbox,
        );
      }
    }

    if (!verified) {
      // Escalate to human
      await slack.postEscalation(
        threadTs,
        config.taskId,
        `Verification failed after ${MAX_VERIFY_ATTEMPTS} attempts`,
      );

      return {
        success: false,
        taskId: config.taskId,
        taskType: "code",
        error: `Verification failed after ${MAX_VERIFY_ATTEMPTS} attempts`,
        durationMs: Date.now() - startTime,
        verifyAttempts,
      };
    }

    // Deliver
    const deliverResult = await runDeliver(
      config,
      github,
      fabric,
      memory,
      slack,
      threadTs,
      planResult.plan,
      implementResult.summary,
    );

    return {
      success: true,
      taskId: config.taskId,
      taskType: "code",
      prUrl: deliverResult.prUrl,
      durationMs: Date.now() - startTime,
      verifyAttempts,
    };
  } catch (error) {
    const message =
      error instanceof Error ? error.message : "Unknown error";

    return {
      success: false,
      taskId: config.taskId,
      taskType: config.taskType,
      error: message,
      durationMs: Date.now() - startTime,
    };
  }
}
