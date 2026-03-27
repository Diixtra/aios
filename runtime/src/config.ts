import { readFile } from "node:fs/promises";
import type { TaskConfig, ToolPolicy } from "./types.js";

/**
 * Default tool policy path inside the K8s pod.
 */
const DEFAULT_POLICY_PATH = "/etc/aios/toolpolicy/policy.json";

/**
 * Loads task configuration from environment variables.
 * Throws if required variables are missing.
 */
export function loadTaskConfig(): TaskConfig {
  const required = (name: string): string => {
    const value = process.env[name];
    if (!value) {
      throw new Error(`Missing required environment variable: ${name}`);
    }
    return value;
  };

  const optional = (name: string): string | undefined => process.env[name];

  const issueNumberStr = optional("AIOS_ISSUE_NUMBER");

  return {
    taskId: required("AIOS_TASK_ID"),
    taskType: required("AIOS_TASK_TYPE") as "code" | "research",
    prompt: required("AIOS_PROMPT"),
    repo: required("AIOS_REPO"),
    issueNumber: issueNumberStr ? parseInt(issueNumberStr, 10) : undefined,
    branch: required("AIOS_BRANCH"),
    slackChannel: required("AIOS_SLACK_CHANNEL"),
    slackThreadTs: optional("AIOS_SLACK_THREAD_TS"),
    memoryUrl: required("AIOS_MEMORY_URL"),
    searchUrl: required("AIOS_SEARCH_URL"),
    workspace: optional("AIOS_WORKSPACE") ?? "/workspace",
  };
}

/**
 * Loads tool policy from a JSON file.
 * Falls back to a restrictive default policy if the file doesn't exist.
 */
export async function loadToolPolicy(
  path: string = DEFAULT_POLICY_PATH,
): Promise<ToolPolicy> {
  try {
    const raw = await readFile(path, "utf-8");
    const parsed = JSON.parse(raw);

    return {
      allowedCommands: parsed.allowedCommands ?? [],
      deniedCommands: parsed.deniedCommands ?? [],
      writablePaths: parsed.writablePaths ?? [],
      readablePaths: parsed.readablePaths ?? [],
    };
  } catch {
    // Missing policy file is a deployment error — deny everything and log loudly
    console.error(
      `FATAL: Failed to read tool policy from ${path}. ` +
        "Falling back to deny-all policy. " +
        "This is a deployment error — ensure the ToolPolicy ConfigMap is mounted.",
    );
    return {
      allowedCommands: [],
      deniedCommands: [],
      writablePaths: [],
      readablePaths: [],
    };
  }
}
