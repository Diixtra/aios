import { describe, it, expect, vi } from "vitest";
import { Sandbox, registerSandbox } from "./index";

describe("Sandbox", () => {
  it("denies disallowed shell commands", () => {
    const sb = new Sandbox({ allowed: ["git status", "npm test"] });
    expect(sb.allow("rm -rf /")).toBe(false);
    expect(sb.allow("git status")).toBe(true);
    expect(sb.allow("git status -uno")).toBe(true); // prefix match
  });

  it("denies an empty command and unrelated prefixes", () => {
    const sb = new Sandbox({ allowed: ["git status"] });
    expect(sb.allow("")).toBe(false);
    expect(sb.allow("gitsomething else")).toBe(false);
    expect(sb.allow("npm test")).toBe(false);
  });

  it("allows everything when allowlist contains '*'", () => {
    const sb = new Sandbox({ allowed: ["*"] });
    expect(sb.allow("anything goes")).toBe(true);
    expect(sb.allow("rm -rf /")).toBe(true);
  });
});

describe("registerSandbox", () => {
  it("wires beforeToolExecute('bash', ...) to deny disallowed commands", () => {
    const handlers: Record<string, (call: { command: string }) => unknown> = {};
    const session = {
      beforeToolExecute: vi.fn(
        (name: string, handler: (call: { command: string }) => unknown) => {
          handlers[name] = handler;
        },
      ),
    } as const;

    registerSandbox(session, { allowed: ["git status"] });

    expect(session.beforeToolExecute).toHaveBeenCalledWith(
      "bash",
      expect.any(Function),
    );
    expect(handlers.bash({ command: "git status" })).toEqual({ allow: true });
    expect(handlers.bash({ command: "rm -rf /" })).toEqual({
      allow: false,
      reason: "sandbox: denied rm -rf /",
    });
  });
});
