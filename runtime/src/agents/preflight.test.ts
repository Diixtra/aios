/**
 * Preflight tests — verify the code-pr agent's pre-run setup:
 *  - Clones the repo via the supplied gh client.
 *  - Fetches the issue title + body.
 *  - Returns the structured handoff to the runPi step.
 */
import { describe, expect, it, vi } from "vitest";
import { preflight } from "./preflight.js";

describe("preflight", () => {
  it("clones the repo and fetches the issue", async () => {
    const fakeGh = {
      cloneRepo: vi.fn(async () => "/tmp/work/aios"),
      getIssue: vi.fn(async () => ({ body: "fix the bug", title: "x" })),
    };
    const out = await preflight({
      repo: "Diixtra/aios",
      issue: 42,
      gh: fakeGh,
    });
    expect(out.repoDir).toBe("/tmp/work/aios");
    expect(out.issueBody).toContain("fix the bug");
    expect(out.issueTitle).toBe("x");
    expect(fakeGh.cloneRepo).toHaveBeenCalledWith("Diixtra/aios");
    expect(fakeGh.getIssue).toHaveBeenCalledWith("Diixtra/aios", 42);
  });

  it("propagates clone errors so the agent fails closed", async () => {
    const fakeGh = {
      cloneRepo: vi.fn(async () => {
        throw new Error("clone failed");
      }),
      getIssue: vi.fn(),
    };
    await expect(
      preflight({ repo: "Diixtra/aios", issue: 1, gh: fakeGh }),
    ).rejects.toThrow("clone failed");
    expect(fakeGh.getIssue).not.toHaveBeenCalled();
  });

  it("propagates issue-fetch errors", async () => {
    const fakeGh = {
      cloneRepo: vi.fn(async () => "/tmp/work/aios"),
      getIssue: vi.fn(async () => {
        throw new Error("issue not found");
      }),
    };
    await expect(
      preflight({ repo: "Diixtra/aios", issue: 999, gh: fakeGh }),
    ).rejects.toThrow("issue not found");
  });
});
