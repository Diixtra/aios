/**
 * runPi — drive the pi coding agent in-process via the pi SDK.
 *
 * Replaces the pre-pivot CLI subprocess approach. The flow is:
 *   1. Build the four-extension adapter (sandbox + MCP + deliver_result;
 *      slack-thread is wired separately because it consumes events via
 *      `AgentSession.subscribe`).
 *   2. Call `sdk.createAgentSession(...)`, passing the extension factory
 *      so the SDK can invoke it with its own `ExtensionAPI`.
 *   3. Wire slack-thread directly against the returned session.
 *   4. `await session.prompt(...)`. The agent must call `deliver_result`
 *      before finishing; if it doesn't, we degrade to a draft result.
 *   5. `dispose()` the session in a `finally` so resources clean up even
 *      when prompt() throws.
 *
 * The `sdk` parameter is injected so tests can stub createAgentSession
 * without pulling in the real model registry / auth layer.
 */
import { buildPiExtensionFactory } from "./pi-extension.js";
import {
  registerSlackThread,
  type SlackThreadOpts,
  type SlackThreadSession,
} from "../../pi-extensions/slack-thread/index.js";
import type { SandboxOpts } from "../../pi-extensions/sandbox/index.js";
import type {
  MCPServerConfig,
  MCPClientFactory,
} from "../../pi-extensions/mcp/index.js";

export interface PiResult {
  branch?: string;
  status: "ready" | "draft" | "error";
  summary: string;
}

/**
 * Minimal session-shaped surface returned by the SDK that runPi consumes.
 * In production this is a real `AgentSession`; in tests it's a fake.
 */
export interface RunPiSessionLike extends SlackThreadSession {
  prompt(text: string): Promise<void>;
  dispose(): void;
}

/**
 * Minimal SDK-shaped surface runPi consumes. We accept any object with a
 * `createAgentSession` function; the production caller passes the real
 * `@mariozechner/pi-coding-agent` module namespace. The resulting session
 * must expose `prompt`, `dispose`, and `subscribe`.
 */
export interface RunPiSdkLike {
  createAgentSession(opts: {
    model: string;
    systemPrompt: string;
    skills: string[];
    extensions: Array<(pi: never) => void | Promise<void>>;
    [k: string]: unknown;
  }): Promise<{ session: RunPiSessionLike; [k: string]: unknown }>;
}

export interface RunPiOpts {
  sdk: RunPiSdkLike;
  model: string;
  systemPrompt: string;
  skills: string[];
  piDir: string;
  prompt: string;
  sandboxOpts: SandboxOpts;
  slack?: SlackThreadOpts;
  mcpServers: MCPServerConfig[];
  /** Test seam — overrides the real MCP client factory. */
  mcpClientFactory?: MCPClientFactory;
}

/**
 * Run pi against the supplied prompt and return the agent's deliver_result
 * payload (or a degraded draft if the agent finished without delivering).
 */
export async function runPi(opts: RunPiOpts): Promise<PiResult> {
  // Pi reads its config + auth bundle out of this directory.
  process.env.PI_CODING_AGENT_DIR = opts.piDir;

  let resolved: PiResult | undefined;
  let resolveResult!: (r: PiResult) => void;
  const resultPromise = new Promise<PiResult>((res) => {
    resolveResult = res;
  });

  const extensionFactory = buildPiExtensionFactory({
    sandbox: opts.sandboxOpts,
    mcpServers: opts.mcpServers,
    mcpClientFactory: opts.mcpClientFactory,
    onDeliverResult: (args) => {
      resolved = args;
      resolveResult(args);
    },
  });

  const { session } = await opts.sdk.createAgentSession({
    model: opts.model,
    systemPrompt: opts.systemPrompt,
    skills: opts.skills,
    extensions: [extensionFactory as unknown as (pi: never) => Promise<void>],
  });

  if (opts.slack) {
    registerSlackThread(session, opts.slack);
  }

  try {
    await session.prompt(opts.prompt);
    if (!resolved) {
      // Agent finished without calling deliver_result — degrade to draft.
      resolveResult({
        status: "draft",
        summary: "agent finished without delivering result",
      });
    }
    return await resultPromise;
  } finally {
    session.dispose();
  }
}
