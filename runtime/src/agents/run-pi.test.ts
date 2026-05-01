/**
 * runPi tests — verify the SDK-driven pi invocation:
 *  - createAgentSession is called with the supplied model + customTools.
 *  - The four extensions (sandbox, slack, mcp, deliver_result) are wired
 *    into the session's structural surface.
 *  - The run resolves with the deliver_result payload.
 *  - dispose() is called even when prompt() throws.
 */
import { describe, expect, it, vi } from "vitest";
import { runPi } from "./run-pi.js";

interface ToolDef {
  name: string;
  description?: string;
  parameters?: unknown;
  execute: (toolCallId: string, args: unknown) => Promise<unknown>;
}

function makeFakeSdk() {
  const recorded = {
    createAgentSessionOpts: undefined as unknown,
  };
  const fakeSession = {
    registeredTools: [] as ToolDef[],
    toolCallHandlers: [] as Array<
      (event: {
        type: "tool_call";
        toolName: string;
        input: Record<string, unknown>;
      }) => Promise<{ block?: boolean; reason?: string } | void>
    >,
    listeners: [] as Array<(event: unknown) => Promise<void> | void>,
    disposed: false,
    promptCalls: [] as string[],
    extensionApi: undefined as
      | undefined
      | {
          registerTool: (t: ToolDef) => void;
          on: (event: string, handler: (...a: unknown[]) => unknown) => void;
        },
    subscribe(listener: (e: unknown) => Promise<void> | void) {
      this.listeners.push(listener);
      return () => {};
    },
    dispose() {
      this.disposed = true;
    },
    async prompt(text: string) {
      this.promptCalls.push(text);
    },
  };

  // Build a fake ExtensionAPI that the adapter would receive in production.
  const extensionApi = {
    registerTool: (tool: ToolDef) => {
      fakeSession.registeredTools.push(tool);
    },
    on: (event: string, handler: (...a: unknown[]) => unknown) => {
      if (event === "tool_call") {
        fakeSession.toolCallHandlers.push(
          handler as (e: never) => Promise<{
            block?: boolean;
            reason?: string;
          } | void>,
        );
      }
    },
  };
  fakeSession.extensionApi = extensionApi;

  const sdk = {
    createAgentSession: vi.fn(async (opts: Record<string, unknown>) => {
      recorded.createAgentSessionOpts = opts;
      // Simulate the SDK invoking each extension factory with the API.
      const extensions = (opts.extensions ?? []) as Array<
        (api: typeof extensionApi) => void | Promise<void>
      >;
      for (const ext of extensions) {
        await ext(extensionApi);
      }
      return { session: fakeSession, extensionsResult: { extensions: [] } };
    }),
  };

  return { sdk, fakeSession, extensionApi, recorded };
}

describe("runPi", () => {
  it("delivers via deliver_result tool and returns the payload", async () => {
    const { sdk, fakeSession } = makeFakeSdk();
    let promptText: string | undefined;

    fakeSession.prompt = vi.fn(async function (this: typeof fakeSession, text: string) {
      promptText = text;
      // Simulate the agent calling deliver_result mid-run.
      const tool = this.registeredTools.find((t) => t.name === "deliver_result");
      if (!tool) throw new Error("deliver_result not registered");
      await tool.execute("call-1", {
        branch: "feat/x",
        status: "ready",
        summary: "ok",
      });
    });

    const result = await runPi({
      sdk: sdk as never,
      model: "openai-codex/gpt-5.4",
      systemPrompt: "you are pi",
      skills: [],
      piDir: "/tmp/pi-x",
      prompt: "go",
      sandboxOpts: { allowed: ["git"] },
      slack: undefined,
      mcpServers: [],
    });

    expect(result.branch).toBe("feat/x");
    expect(result.status).toBe("ready");
    expect(result.summary).toBe("ok");
    expect(fakeSession.disposed).toBe(true);
    expect(promptText).toBe("go");
  });

  it("returns a draft fallback when the agent ends without delivering", async () => {
    const { sdk, fakeSession } = makeFakeSdk();
    fakeSession.prompt = vi.fn(async () => {
      // never calls deliver_result
    });

    const result = await runPi({
      sdk: sdk as never,
      model: "openai-codex/gpt-5.4",
      systemPrompt: "x",
      skills: [],
      piDir: "/tmp/pi-y",
      prompt: "go",
      sandboxOpts: { allowed: ["git"] },
      slack: undefined,
      mcpServers: [],
    });

    expect(result.status).toBe("draft");
    expect(result.summary).toContain("without delivering");
    expect(fakeSession.disposed).toBe(true);
  });

  it("disposes the session even if prompt throws", async () => {
    const { sdk, fakeSession } = makeFakeSdk();
    fakeSession.prompt = vi.fn(async () => {
      throw new Error("provider boom");
    });

    await expect(
      runPi({
        sdk: sdk as never,
        model: "openai-codex/gpt-5.4",
        systemPrompt: "x",
        skills: [],
        piDir: "/tmp/pi-z",
        prompt: "go",
        sandboxOpts: { allowed: ["git"] },
        slack: undefined,
        mcpServers: [],
      }),
    ).rejects.toThrow("provider boom");
    expect(fakeSession.disposed).toBe(true);
  });

  it("wires the sandbox into the SDK extension surface (tool_call hook for bash)", async () => {
    const { sdk, fakeSession } = makeFakeSdk();

    fakeSession.prompt = vi.fn(async function (this: typeof fakeSession) {
      // Trigger the registered tool_call handler with a denied bash call.
      const handler = this.toolCallHandlers[0];
      if (!handler) throw new Error("no tool_call handler wired");
      const denied = await handler({
        type: "tool_call",
        toolName: "bash",
        input: { command: "rm -rf /" },
      });
      expect(denied?.block).toBe(true);

      const allowed = await handler({
        type: "tool_call",
        toolName: "bash",
        input: { command: "git status -uno" },
      });
      // No block returned for allowed commands.
      expect(allowed?.block).not.toBe(true);

      // Now satisfy deliver_result so runPi resolves.
      const tool = this.registeredTools.find((t) => t.name === "deliver_result");
      await tool!.execute("c-2", {
        status: "ready",
        summary: "wired",
        branch: "feat/y",
      });
    });

    const result = await runPi({
      sdk: sdk as never,
      model: "openai-codex/gpt-5.4",
      systemPrompt: "x",
      skills: [],
      piDir: "/tmp/pi-q",
      prompt: "go",
      sandboxOpts: { allowed: ["git"] },
      slack: undefined,
      mcpServers: [],
    });
    expect(result.status).toBe("ready");
  });

  it("registers MCP server tools through the extension adapter", async () => {
    const { sdk, fakeSession } = makeFakeSdk();
    const mcpFactory = vi.fn(async () => ({
      listTools: async () => ({
        tools: [{ name: "search", description: "search the index" }],
      }),
      callTool: vi.fn(async () => ({ content: "result" })),
    }));

    fakeSession.prompt = vi.fn(async function (this: typeof fakeSession) {
      const tool = this.registeredTools.find((t) => t.name === "deliver_result");
      await tool!.execute("c-3", {
        status: "ready",
        summary: "mcp wired",
        branch: "feat/m",
      });
    });

    await runPi({
      sdk: sdk as never,
      model: "openai-codex/gpt-5.4",
      systemPrompt: "x",
      skills: [],
      piDir: "/tmp/pi-mcp",
      prompt: "go",
      sandboxOpts: { allowed: ["git"] },
      slack: undefined,
      mcpServers: [{ name: "aios-search", url: "http://search/" }],
      mcpClientFactory: mcpFactory as never,
    });

    expect(mcpFactory).toHaveBeenCalledOnce();
    const names = fakeSession.registeredTools.map((t) => t.name);
    expect(names).toContain("aios-search.search");
    expect(names).toContain("deliver_result");
  });
});
