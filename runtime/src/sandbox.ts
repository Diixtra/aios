import { execFile } from "node:child_process";
import type { ToolPolicy, CommandResult } from "./types.js";

/**
 * Result of a command or file access validation.
 */
export interface ValidationResult {
  allowed: boolean;
  reason?: string;
}

/**
 * Shell metacharacters that are blocked to prevent injection.
 */
const BLOCKED_METACHARACTERS = /[;|&`$(){}><#~\n\r]/;

/**
 * Path traversal pattern.
 */
const PATH_TRAVERSAL = /\.\.\//;

/**
 * Sandbox enforces tool policy for command execution and file access.
 * Uses deny-by-default: commands must match an allowed prefix and not
 * match any denied prefix. Denied entries take priority over allowed.
 */
export class Sandbox {
  constructor(private readonly policy: ToolPolicy) {}

  /**
   * Validates whether a command is allowed to execute.
   * Checks in order:
   * 1. Block shell metacharacters
   * 2. Block path traversal
   * 3. Check denied list (deny overrides allow)
   * 4. Check allowed list by prefix matching
   */
  validateCommand(command: string): ValidationResult {
    const trimmed = command.trim();

    if (!trimmed) {
      return { allowed: false, reason: "Empty command" };
    }

    // Block shell metacharacters
    if (BLOCKED_METACHARACTERS.test(trimmed)) {
      const match = trimmed.match(BLOCKED_METACHARACTERS);
      return {
        allowed: false,
        reason: `Blocked shell metacharacter: "${match?.[0]}"`,
      };
    }

    // Block path traversal
    if (PATH_TRAVERSAL.test(trimmed)) {
      return { allowed: false, reason: "Path traversal not allowed: ../" };
    }

    // Check denied list first (deny overrides allow)
    for (const denied of this.policy.deniedCommands) {
      if (trimmed.startsWith(denied)) {
        return {
          allowed: false,
          reason: `Command matches denied prefix: "${denied}"`,
        };
      }
    }

    // Check allowed list by prefix matching
    for (const allowed of this.policy.allowedCommands) {
      if (trimmed.startsWith(allowed)) {
        return { allowed: true };
      }
    }

    // Deny by default
    return {
      allowed: false,
      reason: "Command does not match any allowed prefix",
    };
  }

  /**
   * Validates whether file access is permitted.
   * @param filePath - The absolute path to check
   * @param mode - "read" or "write"
   */
  validateFileAccess(
    filePath: string,
    mode: "read" | "write",
  ): ValidationResult {
    const patterns =
      mode === "write" ? this.policy.writablePaths : this.policy.readablePaths;

    // Block path traversal in file paths
    if (PATH_TRAVERSAL.test(filePath)) {
      return { allowed: false, reason: "Path traversal not allowed: ../" };
    }

    for (const pattern of patterns) {
      if (matchGlob(pattern, filePath)) {
        return { allowed: true };
      }
    }

    return {
      allowed: false,
      reason: `Path "${filePath}" does not match any ${mode}able pattern`,
    };
  }
}

/**
 * Execute a command after validating it against the sandbox policy.
 * Rejects commands that fail validation without spawning a process.
 */
export function sandboxedExec(
  sandbox: Sandbox,
  command: string,
  args: string[],
  cwd: string,
): Promise<CommandResult> {
  const fullCommand = `${command} ${args.join(" ")}`.trim();
  const validation = sandbox.validateCommand(fullCommand);

  if (!validation.allowed) {
    return Promise.resolve({
      exitCode: 126,
      stdout: "",
      stderr: `Sandbox blocked command: ${validation.reason}`,
    });
  }

  return new Promise((resolve) => {
    execFile(
      command,
      args,
      { cwd, maxBuffer: 10 * 1024 * 1024 },
      (error, stdout, stderr) => {
        resolve({
          exitCode: error?.code !== undefined ? (error.code as number) : 0,
          stdout: stdout ?? "",
          stderr: stderr ?? "",
        });
      },
    );
  });
}

/**
 * Simple glob matcher supporting ** (any path) and * (single segment).
 */
export function matchGlob(pattern: string, path: string): boolean {
  // Convert glob to regex
  let regex = "^";
  let i = 0;
  while (i < pattern.length) {
    if (pattern[i] === "*" && pattern[i + 1] === "*") {
      regex += ".*";
      i += 2;
      // Skip trailing slash after **
      if (pattern[i] === "/") {
        i++;
      }
    } else if (pattern[i] === "*") {
      regex += "[^/]*";
      i++;
    } else if (pattern[i] === "?") {
      regex += "[^/]";
      i++;
    } else if (".+^${}()|[]\\".includes(pattern[i])) {
      regex += "\\" + pattern[i];
      i++;
    } else {
      regex += pattern[i];
      i++;
    }
  }
  regex += "$";
  return new RegExp(regex).test(path);
}
