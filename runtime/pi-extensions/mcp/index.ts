/**
 * Pi MCP bridge extension — exposes remote MCP server tools as pi tools.
 *
 * For each configured `{name, url}` server, the bridge:
 *   1. Opens an MCP client (via the supplied factory).
 *   2. Calls `client.listTools()` to enumerate available tools.
 *   3. Registers each tool against the pi session under a namespaced name
 *      (`<server>.<tool>`) so multiple servers can expose tools with
 *      identical local names without colliding.
 *
 * The handler proxies invocations back to `client.callTool({ name, arguments })`
 * using the unprefixed tool name (the prefix is only for the session-side
 * registry; the remote server doesn't know about it).
 *
 * The `clientFactory` argument is optional in production usage — when omitted
 * a default factory creates real clients via `@modelcontextprotocol/sdk`. In
 * tests, callers pass a fake factory to avoid real network I/O.
 */

import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StreamableHTTPClientTransport } from "@modelcontextprotocol/sdk/client/streamableHttp.js";

export interface MCPServerConfig {
  name: string;
  url: string;
}

/**
 * Subset of the @modelcontextprotocol/sdk Client surface we depend on.
 *
 * The real SDK's `listTools` resolves to an object with a `.tools` array.
 * `callTool` takes `{ name, arguments }` and resolves to `{ content, isError? }`.
 */
export interface MCPClientLike {
  listTools(): Promise<{
    tools: Array<{
      name: string;
      description?: string;
      inputSchema?: unknown;
    }>;
  }>;
  callTool(params: {
    name: string;
    arguments?: unknown;
  }): Promise<{ content: unknown; isError?: boolean; [k: string]: unknown }>;
}

export type MCPClientFactory = (
  server: MCPServerConfig,
) => MCPClientLike | Promise<MCPClientLike>;

export interface MCPToolHandlerArgs {
  [k: string]: unknown;
}

/**
 * Minimal pi-session-shaped registry receiver. Real production code passes the
 * pi `ExtensionAPI` (which has `registerTool`); tests pass a `vi.fn()` mock.
 */
export interface MCPSession {
  registerTool(tool: {
    name: string;
    description?: string;
    inputSchema?: unknown;
    handler: (args: MCPToolHandlerArgs) => Promise<unknown>;
  }): void;
}

/**
 * Default client factory — opens a real MCP client over StreamableHTTP transport.
 * Exported so callers can compose it (e.g. wrap with auth headers) and so tests
 * can assert on its identity if needed.
 */
export const defaultMCPClientFactory: MCPClientFactory = async (server) => {
  const transport = new StreamableHTTPClientTransport(new URL(server.url));
  const client = new Client(
    { name: `aios-runtime/${server.name}`, version: "0.1.0" },
    { capabilities: {} },
  );
  await client.connect(transport);
  return client as unknown as MCPClientLike;
};

/**
 * Register every tool from each MCP server against the pi session.
 *
 * Returns a promise that resolves once all servers' tools are listed and
 * wired. Errors from any server short-circuit the entire registration so
 * the caller learns about misconfigured servers up front.
 */
export async function registerMCP(
  session: MCPSession,
  servers: MCPServerConfig[],
  clientFactory: MCPClientFactory = defaultMCPClientFactory,
): Promise<void> {
  for (const server of servers) {
    const client = await clientFactory(server);
    const { tools } = await client.listTools();
    for (const tool of tools) {
      const localName = tool.name;
      session.registerTool({
        name: `${server.name}.${localName}`,
        description: tool.description,
        inputSchema: tool.inputSchema,
        handler: async (args) => {
          return client.callTool({ name: localName, arguments: args });
        },
      });
    }
  }
}
