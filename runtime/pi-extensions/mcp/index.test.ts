import { describe, it, expect, vi } from "vitest";
import { registerMCP, type MCPClientLike } from "./index";

/**
 * Fake MCP client matching the @modelcontextprotocol/sdk Client API surface used here:
 * - listTools(): Promise<{ tools: [...] }>   (NOT a bare array)
 * - callTool({ name, arguments }): Promise<{ content, isError? }>
 * The plan's pseudocode showed `listTools` returning a bare array; the real SDK
 * wraps tools in `{ tools: [...] }` so tests use that shape.
 */
function makeFakeClient(toolName = "search"): MCPClientLike {
  return {
    listTools: vi.fn(async () => ({
      tools: [
        {
          name: toolName,
          description: "Search vault",
          inputSchema: { type: "object", properties: { q: { type: "string" } } },
        },
      ],
    })),
    callTool: vi.fn(async (params: { name: string; arguments?: unknown }) => ({
      content: [{ type: "text" as const, text: `called ${params.name}` }],
    })),
  };
}

describe("registerMCP", () => {
  it("registers each server's tools against the session with mcp-prefixed names", async () => {
    const fakeSession = { registerTool: vi.fn() };
    const fakeClient = makeFakeClient("search");

    await registerMCP(
      fakeSession,
      [{ name: "aios-search", url: "http://x" }],
      () => fakeClient,
    );

    expect(fakeClient.listTools).toHaveBeenCalledTimes(1);
    expect(fakeSession.registerTool).toHaveBeenCalledTimes(1);
    const def = (fakeSession.registerTool as ReturnType<typeof vi.fn>).mock
      .calls[0][0];
    expect(def.name).toBe("aios-search.search");
    expect(def.description).toBe("Search vault");
    expect(def.inputSchema).toEqual({
      type: "object",
      properties: { q: { type: "string" } },
    });
    expect(typeof def.handler).toBe("function");
  });

  it("the registered tool handler proxies to client.callTool with the unprefixed name", async () => {
    const fakeSession = { registerTool: vi.fn() };
    const fakeClient = makeFakeClient("search");

    await registerMCP(
      fakeSession,
      [{ name: "aios-search", url: "http://x" }],
      () => fakeClient,
    );

    const def = (fakeSession.registerTool as ReturnType<typeof vi.fn>).mock
      .calls[0][0];
    const result = await def.handler({ q: "hello" });
    expect(fakeClient.callTool).toHaveBeenCalledWith({
      name: "search",
      arguments: { q: "hello" },
    });
    expect(result).toEqual({
      content: [{ type: "text", text: "called search" }],
    });
  });

  it("registers tools from multiple servers without name collisions", async () => {
    const fakeSession = { registerTool: vi.fn() };
    const clientA = makeFakeClient("query");
    const clientB = makeFakeClient("query");
    const factory = vi.fn((server: { name: string; url: string }) =>
      server.name === "alpha" ? clientA : clientB,
    );

    await registerMCP(
      fakeSession,
      [
        { name: "alpha", url: "http://a" },
        { name: "beta", url: "http://b" },
      ],
      factory,
    );

    const names = (fakeSession.registerTool as ReturnType<typeof vi.fn>).mock
      .calls.map((c) => c[0].name);
    expect(names).toEqual(["alpha.query", "beta.query"]);
    expect(factory).toHaveBeenCalledTimes(2);
  });
});
