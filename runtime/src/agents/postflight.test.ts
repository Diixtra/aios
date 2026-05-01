/**
 * Postflight tests — verify the code-pr agent opens a PR matching the pi
 * deliver_result status:
 *  - status="ready"  -> regular (non-draft) PR.
 *  - status="draft"  -> draft PR (work-in-progress).
 *  - missing branch  -> hard error (we never pi-deliver without a branch).
 *  - status="error"  -> no PR opened, error surfaced to caller.
 */
import { describe, expect, it, vi } from "vitest";
import { postflight } from "./postflight.js";

describe("postflight", () => {
  it("opens a draft PR for status=draft", async () => {
    const gh = {
      openPR: vi.fn(async () => ({ url: "https://gh/pr/1" })),
    };
    const out = await postflight({
      result: { branch: "feat/x", status: "draft", summary: "wip" },
      repo: "x/y",
      issue: 1,
      gh,
    });
    expect(out.prUrl).toBe("https://gh/pr/1");
    expect(gh.openPR).toHaveBeenCalledWith(
      expect.objectContaining({
        repo: "x/y",
        head: "feat/x",
        draft: true,
      }),
    );
    const call = gh.openPR.mock.calls[0]![0]!;
    expect(call.title).toContain("#1");
    expect(call.title).toContain("wip");
    expect(call.body).toContain("Closes #1");
  });

  it("opens a real (non-draft) PR for status=ready", async () => {
    const gh = {
      openPR: vi.fn(async () => ({ url: "https://gh/pr/2" })),
    };
    const out = await postflight({
      result: { branch: "feat/y", status: "ready", summary: "ship it" },
      repo: "x/y",
      issue: 2,
      gh,
    });
    expect(out.prUrl).toBe("https://gh/pr/2");
    expect(gh.openPR).toHaveBeenCalledWith(
      expect.objectContaining({ draft: false, head: "feat/y" }),
    );
  });

  it("throws when pi did not return a branch (no PR can be opened)", async () => {
    const gh = { openPR: vi.fn() };
    await expect(
      postflight({
        result: { status: "draft", summary: "no branch" },
        repo: "x/y",
        issue: 3,
        gh,
      }),
    ).rejects.toThrow(/no branch/);
    expect(gh.openPR).not.toHaveBeenCalled();
  });

  it("surfaces status=error without opening a PR", async () => {
    const gh = { openPR: vi.fn() };
    await expect(
      postflight({
        result: { status: "error", summary: "build failed", branch: "feat/z" },
        repo: "x/y",
        issue: 4,
        gh,
      }),
    ).rejects.toThrow(/build failed/);
    expect(gh.openPR).not.toHaveBeenCalled();
  });
});
