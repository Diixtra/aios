import type { TaskConfig, CommandResult } from "../types.js";
import type { FabricRunner } from "../fabric.js";
import type { SlackNotifier } from "../slack.js";
import { type Sandbox, sandboxedExec } from "../sandbox.js";

export interface VerifyResult {
  /** Whether verification passed */
  passed: boolean;
  /** Test execution result */
  testResult?: CommandResult;
  /** Fabric review output */
  reviewOutput?: string;
  /** Reasons for failure */
  failureReasons: string[];
}

/**
 * Verify stage: runs tests, fabric review, and self-review.
 */
export async function runVerify(
  config: TaskConfig,
  fabric: FabricRunner,
  slack: SlackNotifier,
  threadTs: string,
  attempt: number,
  sandbox: Sandbox,
): Promise<VerifyResult> {
  const failureReasons: string[] = [];

  await slack.postToThread(
    config.slackChannel,
    threadTs,
    `:test_tube: Running verification (attempt ${attempt})...`,
  );

  // Run tests
  const testResult = await sandboxedExec(
    sandbox,
    "npm",
    ["test"],
    config.workspace,
  );

  if (testResult.exitCode !== 0) {
    failureReasons.push(`Tests failed (exit code ${testResult.exitCode})`);
  }

  // Run fabric code review
  const diffResult = await sandboxedExec(
    sandbox,
    "git",
    ["diff", "--cached"],
    config.workspace,
  );

  let reviewOutput: string | undefined;
  if (diffResult.stdout.trim()) {
    const review = await fabric.run("review", diffResult.stdout);
    reviewOutput = review.stdout;

    if (review.exitCode !== 0) {
      failureReasons.push("Fabric review pattern failed");
    }
  }

  const passed = failureReasons.length === 0;

  if (passed) {
    await slack.postToThread(
      config.slackChannel,
      threadTs,
      `:white_check_mark: Verification passed on attempt ${attempt}.`,
    );
  } else {
    await slack.postToThread(
      config.slackChannel,
      threadTs,
      `:x: Verification failed (attempt ${attempt}): ${failureReasons.join(", ")}`,
    );
  }

  return {
    passed,
    testResult,
    reviewOutput,
    failureReasons,
  };
}
