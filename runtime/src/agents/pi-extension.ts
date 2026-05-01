/**
 * Pi extension adapter for the code-pr agent.
 *
 * The four pi extensions (sandbox, slack-thread, mcp, fabric-skill) shipped
 * in C5-C8 each declared a small structural session interface so they could
 * be unit-tested in isolation. The actual pi SDK exposes:
 *
 *   - `ExtensionAPI` (passed to extension factories): `registerTool`,
 *     `on("tool_call", handler)` returning `{block?, reason?}`, ...
 *   - `AgentSession` (returned from `createAgentSession`): `subscribe`,
 *     `prompt`, `dispose`, ...
 *
 * This file bridges the two. `buildPiExtensionFactory` returns a single
 * `ExtensionFactory` that, when invoked by the SDK with an `ExtensionAPI`:
 *   - Wires the bash sandbox via `pi.on("tool_call", ...)`.
 *   - Registers each MCP server's tools via `pi.registerTool(...)`.
 *   - Registers the `deliver_result` synthesis tool used by runPi to
 *     surface the agent's final payload.
 *
 * Slack-thread is NOT registered here because it consumes events via
 * `AgentSession.subscribe` rather than the extension API. The caller
 * registers slack against the live session after `createAgentSession`
 * returns.
 *
 * Fabric-skill is also NOT registered here — it's a pre-session helper that
 * resolves on-disk skill paths, which are passed to the SDK via the
 * `resourceLoader.additionalSkillPaths` option before the session is built.
 */

import {
  registerSandbox,
  type SandboxOpts,
  type SandboxSession,
} from "../../pi-extensions/sandbox/index.js";
import {
  registerMCP,
  type MCPClientFactory,
  type MCPServerConfig,
  type MCPSession,
} from "../../pi-extensions/mcp/index.js";

/**
 * Subset of pi's `ExtensionAPI` we depend on.
 *
 * We intentionally keep this surface small so the adapter (and its tests)
 * don't have to mock the full ExtensionAPI shape. The relevant methods are
 * `registerTool` (for MCP-bridged tools and deliver_result) and
 * `on("tool_call", ...)` (for the sandbox interception).
 */
export interface PiExtensionAPILike {
  registerTool(tool: {
    name: string;
    description?: string;
    parameters?: unknown;
    /** Pi's ToolDefinition.execute signature (toolCallId, params, ...). */
    execute: (
      toolCallId: string,
      params: Record<string, unknown>,
    ) => Promise<unknown>;
    [k: string]: unknown;
  }): void;
  on(
    event: "tool_call",
    handler: (event: {
      type: "tool_call";
      toolName: string;
      input: Record<string, unknown>;
    }) => Promise<{ block?: boolean; reason?: string } | void> | {
      block?: boolean;
      reason?: string;
    } | void,
  ): void;
}

/** Tool definition used for the deliver_result synthesis tool. */
export interface DeliverResultArgs {
  status: "ready" | "draft" | "error";
  summary: string;
  branch?: string;
}

export interface PiExtensionFactoryOpts {
  sandbox: SandboxOpts;
  mcpServers: MCPServerConfig[];
  /** Override for tests / dependency injection. */
  mcpClientFactory?: MCPClientFactory;
  /** Called when the agent invokes the deliver_result tool. */
  onDeliverResult: (args: DeliverResultArgs) => void;
}

/**
 * Build an `ExtensionFactory`-shaped function. The SDK invokes it with the
 * real `ExtensionAPI`; we structurally accept anything compatible with
 * `PiExtensionAPILike` so the adapter is testable without the SDK's full
 * dependency graph.
 */
export function buildPiExtensionFactory(
  opts: PiExtensionFactoryOpts,
): (pi: PiExtensionAPILike) => Promise<void> {
  return async (pi: PiExtensionAPILike) => {
    // 1. Sandbox — adapt pi.on("tool_call") into the SandboxSession shape.
    const sandboxSession: SandboxSession = {
      beforeToolExecute(toolName, handler) {
        pi.on("tool_call", (event) => {
          if (event.toolName !== toolName) return;
          const input = event.input as { command?: string };
          const command = typeof input.command === "string" ? input.command : "";
          const verdict = handler({ command });
          if (!verdict.allow) {
            return { block: true, reason: verdict.reason };
          }
        });
      },
    };
    registerSandbox(sandboxSession, opts.sandbox);

    // 2. MCP — adapt pi.registerTool into the MCPSession shape.
    const mcpSession: MCPSession = {
      registerTool(tool) {
        pi.registerTool({
          name: tool.name,
          description: tool.description,
          parameters: tool.inputSchema,
          execute: async (_toolCallId, params) => {
            return tool.handler(params);
          },
        });
      },
    };
    await registerMCP(mcpSession, opts.mcpServers, opts.mcpClientFactory);

    // 3. deliver_result — synthesis tool that surfaces the agent's verdict.
    pi.registerTool({
      name: "deliver_result",
      description:
        "Deliver the final agent result. Call this once when the implementation is complete.",
      parameters: {
        type: "object",
        properties: {
          branch: {
            type: "string",
            description: "Git branch the implementation was committed to.",
          },
          status: {
            type: "string",
            enum: ["ready", "draft", "error"],
            description:
              "ready = open a real PR; draft = open as draft; error = fail.",
          },
          summary: {
            type: "string",
            description: "Short description of what was done (used as PR body).",
          },
        },
        required: ["status", "summary"],
      },
      execute: async (_toolCallId, params) => {
        const args: DeliverResultArgs = {
          status: params.status as DeliverResultArgs["status"],
          summary: String(params.summary ?? ""),
          branch: typeof params.branch === "string" ? params.branch : undefined,
        };
        opts.onDeliverResult(args);
        return { ok: true };
      },
    });
  };
}
