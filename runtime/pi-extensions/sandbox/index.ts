/**
 * Pi sandbox extension — allowlist-based bash command gating.
 *
 * Wires a `beforeToolExecute("bash", ...)` hook on the supplied session so the
 * coding agent can only run shell commands that match an allowlist entry
 * (exact match or prefix-match with a trailing space).
 *
 * Note on the session shape: this file deliberately accepts a structural
 * `SandboxSession` interface rather than the SDK's `AgentSession` class.
 * `AgentSession` does not expose `beforeToolExecute` directly — that hook
 * lives on the pi `ExtensionAPI`. The agent-entrypoint wiring (Tasks C9/C10)
 * is responsible for adapting `AgentSession` events into a `SandboxSession`
 * shape (e.g. by registering a small Extension that proxies a single
 * `tool_call` handler, or by passing the ExtensionAPI here directly).
 */

export interface SandboxOpts {
  /**
   * List of command prefixes the agent is permitted to run via bash.
   * - Exact equality matches.
   * - A prefix followed by a space matches (e.g. "git status" allows "git status -uno").
   * - The literal entry "*" allows everything (escape hatch for trusted runs).
   */
  allowed: string[];
}

export interface SandboxBeforeToolResult {
  allow: boolean;
  reason?: string;
}

/** Minimal session-like interface used by registerSandbox. */
export interface SandboxSession {
  beforeToolExecute(
    toolName: "bash",
    handler: (call: { command: string }) => SandboxBeforeToolResult,
  ): void;
}

/** Allowlist matcher for bash commands. */
export class Sandbox {
  constructor(private readonly opts: SandboxOpts) {}

  /**
   * Returns true if the command is on the allowlist.
   * Matches exact strings or prefixes followed by a space.
   * The literal entry "*" is a wildcard that allows everything.
   */
  allow(command: string): boolean {
    if (!command) return false;
    return this.opts.allowed.some(
      (a) => a === "*" || command === a || command.startsWith(a + " "),
    );
  }
}

/**
 * Register the sandbox against a pi session.
 *
 * Wires a `beforeToolExecute("bash", ...)` hook that returns
 * `{ allow: true }` for permitted commands and
 * `{ allow: false, reason }` for denied ones.
 */
export function registerSandbox(
  session: SandboxSession,
  opts: SandboxOpts,
): Sandbox {
  const sb = new Sandbox(opts);
  session.beforeToolExecute("bash", (call) => {
    if (!sb.allow(call.command)) {
      return { allow: false, reason: `sandbox: denied ${call.command}` };
    }
    return { allow: true };
  });
  return sb;
}
