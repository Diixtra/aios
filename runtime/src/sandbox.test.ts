import { describe, it, expect } from "vitest";
import { Sandbox, matchGlob } from "./sandbox.js";
import type { ToolPolicy } from "./types.js";

const testPolicy: ToolPolicy = {
  allowedCommands: ["git ", "npm ", "npx ", "node ", "cat "],
  deniedCommands: ["rm -rf /", "sudo ", "git push --force"],
  writablePaths: ["/workspace/**"],
  readablePaths: ["/workspace/**", "/etc/aios/**"],
};

describe("Sandbox", () => {
  const sandbox = new Sandbox(testPolicy);

  describe("validateCommand", () => {
    it("allows commands matching allowed prefixes", () => {
      expect(sandbox.validateCommand("git status")).toEqual({ allowed: true });
      expect(sandbox.validateCommand("npm install")).toEqual({ allowed: true });
      expect(sandbox.validateCommand("npx vitest run")).toEqual({
        allowed: true,
      });
      expect(sandbox.validateCommand("node dist/index.js")).toEqual({
        allowed: true,
      });
    });

    it("denies commands matching denied prefixes", () => {
      const result = sandbox.validateCommand("rm -rf /");
      expect(result.allowed).toBe(false);
      expect(result.reason).toContain("denied prefix");
    });

    it("denies sudo commands", () => {
      const result = sandbox.validateCommand("sudo apt install something");
      expect(result.allowed).toBe(false);
      expect(result.reason).toContain("denied prefix");
    });

    it("deny overrides allow", () => {
      const result = sandbox.validateCommand("git push --force main");
      expect(result.allowed).toBe(false);
      expect(result.reason).toContain("denied prefix");
    });

    it("denies commands not in allowed list", () => {
      const result = sandbox.validateCommand("curl http://evil.com");
      expect(result.allowed).toBe(false);
      expect(result.reason).toContain("does not match any allowed prefix");
    });

    it("blocks shell metacharacter: semicolon", () => {
      const result = sandbox.validateCommand("git status; rm -rf /");
      expect(result.allowed).toBe(false);
      expect(result.reason).toContain("metacharacter");
    });

    it("blocks shell metacharacter: pipe", () => {
      const result = sandbox.validateCommand("git log | head");
      expect(result.allowed).toBe(false);
      expect(result.reason).toContain("metacharacter");
    });

    it("blocks shell metacharacter: ampersand", () => {
      const result = sandbox.validateCommand("git status && rm -rf /");
      expect(result.allowed).toBe(false);
      expect(result.reason).toContain("metacharacter");
    });

    it("blocks shell metacharacter: backtick", () => {
      const result = sandbox.validateCommand("git commit -m `whoami`");
      expect(result.allowed).toBe(false);
      expect(result.reason).toContain("metacharacter");
    });

    it("blocks shell metacharacter: dollar sign", () => {
      const result = sandbox.validateCommand("git commit -m $USER");
      expect(result.allowed).toBe(false);
      expect(result.reason).toContain("metacharacter");
    });

    it("blocks shell metacharacter: parentheses", () => {
      const result = sandbox.validateCommand("git commit -m $(whoami)");
      expect(result.allowed).toBe(false);
      expect(result.reason).toContain("metacharacter");
    });

    it("blocks path traversal", () => {
      const result = sandbox.validateCommand("cat ../../../etc/passwd");
      expect(result.allowed).toBe(false);
      expect(result.reason).toContain("Path traversal");
    });

    it("denies empty commands", () => {
      const result = sandbox.validateCommand("");
      expect(result.allowed).toBe(false);
      expect(result.reason).toContain("Empty command");
    });

    it("trims whitespace before validation", () => {
      expect(sandbox.validateCommand("  git status  ")).toEqual({
        allowed: true,
      });
    });
  });

  describe("validateFileAccess", () => {
    it("allows reading files in readable paths", () => {
      expect(
        sandbox.validateFileAccess("/workspace/src/index.ts", "read"),
      ).toEqual({ allowed: true });
      expect(
        sandbox.validateFileAccess("/etc/aios/toolpolicy/policy.json", "read"),
      ).toEqual({ allowed: true });
    });

    it("allows writing files in writable paths", () => {
      expect(
        sandbox.validateFileAccess("/workspace/src/index.ts", "write"),
      ).toEqual({ allowed: true });
    });

    it("denies writing to read-only paths", () => {
      const result = sandbox.validateFileAccess(
        "/etc/aios/toolpolicy/policy.json",
        "write",
      );
      expect(result.allowed).toBe(false);
      expect(result.reason).toContain("does not match any writeable pattern");
    });

    it("denies access to paths outside allowed patterns", () => {
      const result = sandbox.validateFileAccess("/etc/passwd", "read");
      expect(result.allowed).toBe(false);
    });

    it("blocks path traversal in file access", () => {
      const result = sandbox.validateFileAccess(
        "/workspace/../etc/passwd",
        "read",
      );
      expect(result.allowed).toBe(false);
      expect(result.reason).toContain("Path traversal");
    });
  });
});

describe("matchGlob", () => {
  it("matches ** patterns", () => {
    expect(matchGlob("/workspace/**", "/workspace/src/index.ts")).toBe(true);
    expect(matchGlob("/workspace/**", "/workspace/deep/nested/file.ts")).toBe(
      true,
    );
  });

  it("matches * patterns for single segments", () => {
    expect(matchGlob("/workspace/*.ts", "/workspace/index.ts")).toBe(true);
    expect(matchGlob("/workspace/*.ts", "/workspace/src/index.ts")).toBe(false);
  });

  it("rejects non-matching paths", () => {
    expect(matchGlob("/workspace/**", "/etc/passwd")).toBe(false);
    expect(matchGlob("/workspace/*.ts", "/workspace/index.js")).toBe(false);
  });
});
