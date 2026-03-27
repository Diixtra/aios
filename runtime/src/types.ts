/**
 * Configuration for an AIOS task, loaded from environment variables.
 */
export interface TaskConfig {
  /** Unique task identifier */
  taskId: string;
  /** Task type: "code" for coding tasks, "research" for research tasks */
  taskType: "code" | "research";
  /** The prompt/instructions for the task */
  prompt: string;
  /** GitHub repository in owner/repo format */
  repo: string;
  /** GitHub issue number, if applicable */
  issueNumber?: number;
  /** Git branch to work on */
  branch: string;
  /** Slack channel ID for notifications */
  slackChannel: string;
  /** Slack thread timestamp for threaded replies */
  slackThreadTs?: string;
  /** Memory MCP server URL */
  memoryUrl: string;
  /** AIOS search service URL */
  searchUrl: string;
  /** Workspace directory for file operations */
  workspace: string;
}

/**
 * Fabric-ai pattern names used in different pipeline stages.
 */
export interface FabricPatterns {
  /** Pattern for understanding/analyzing issues */
  understand: string;
  /** Pattern for creating implementation plans */
  plan: string;
  /** Pattern for code review */
  review: string;
  /** Pattern for generating PR descriptions */
  prDescription: string;
  /** Pattern for research tasks */
  research: string;
}

/**
 * Result from a completed pipeline execution.
 */
export interface PipelineResult {
  /** Whether the pipeline completed successfully */
  success: boolean;
  /** The task ID */
  taskId: string;
  /** The task type */
  taskType: "code" | "research";
  /** URL of created PR (code tasks) */
  prUrl?: string;
  /** Path to output files (research tasks) */
  outputPath?: string;
  /** Error message if failed */
  error?: string;
  /** Duration in milliseconds */
  durationMs: number;
  /** Number of verify attempts used */
  verifyAttempts?: number;
}

/**
 * Result from running a shell command.
 */
export interface CommandResult {
  /** Process exit code */
  exitCode: number;
  /** Combined stdout output */
  stdout: string;
  /** Combined stderr output */
  stderr: string;
}

/**
 * Tool policy defining allowed and denied commands and file access patterns.
 * Loaded from /etc/aios/toolpolicy/policy.json in the pod.
 */
export interface ToolPolicy {
  /** List of allowed command prefixes (e.g., ["git ", "npm ", "npx "]) */
  allowedCommands: string[];
  /** List of denied command prefixes (checked first, overrides allow) */
  deniedCommands: string[];
  /** Glob patterns for writable paths */
  writablePaths: string[];
  /** Glob patterns for readable paths */
  readablePaths: string[];
}
