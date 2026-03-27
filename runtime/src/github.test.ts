import { describe, it, expect, vi, beforeEach } from "vitest";
import { GitHubClient } from "./github.js";
import * as childProcess from "node:child_process";

vi.mock("node:child_process");

function mockGh(stdout: string, stderr: string = "", exitCode: number = 0) {
  vi.mocked(childProcess.execFile).mockImplementation(
    (_cmd: any, _args: any, _opts: any, callback: any) => {
      const cb = typeof _opts === "function" ? _opts : callback;
      process.nextTick(() => {
        if (exitCode !== 0) {
          const err = Object.assign(new Error("gh failed"), { code: exitCode });
          cb(err, stdout, stderr);
        } else {
          cb(null, stdout, stderr);
        }
      });
      return {} as any;
    },
  );
}

describe("GitHubClient", () => {
  let client: GitHubClient;

  beforeEach(() => {
    vi.clearAllMocks();
    client = new GitHubClient("owner/repo");
  });

  describe("createPR", () => {
    it("creates a PR and returns the URL", async () => {
      mockGh("https://github.com/owner/repo/pull/42\n");

      const url = await client.createPR("feat: add feature", "Description");

      expect(url).toBe("https://github.com/owner/repo/pull/42");
      expect(childProcess.execFile).toHaveBeenCalledWith(
        "gh",
        [
          "pr",
          "create",
          "--repo",
          "owner/repo",
          "--title",
          "feat: add feature",
          "--body",
          "Description",
        ],
        expect.any(Object),
        expect.any(Function),
      );
    });

    it("throws on failure", async () => {
      mockGh("", "authentication required", 1);

      await expect(
        client.createPR("title", "body"),
      ).rejects.toThrow("Failed to create PR");
    });
  });

  describe("updateIssueLabel", () => {
    it("adds a label to an issue", async () => {
      mockGh("");

      await client.updateIssueLabel(10, "in-progress");

      expect(childProcess.execFile).toHaveBeenCalledWith(
        "gh",
        [
          "issue",
          "edit",
          "10",
          "--repo",
          "owner/repo",
          "--add-label",
          "in-progress",
        ],
        expect.any(Object),
        expect.any(Function),
      );
    });

    it("throws on failure", async () => {
      mockGh("", "not found", 1);

      await expect(
        client.updateIssueLabel(999, "label"),
      ).rejects.toThrow("Failed to update issue label");
    });
  });

  describe("closeIssue", () => {
    it("closes an issue", async () => {
      mockGh("");

      await client.closeIssue(10);

      expect(childProcess.execFile).toHaveBeenCalledWith(
        "gh",
        ["issue", "close", "10", "--repo", "owner/repo"],
        expect.any(Object),
        expect.any(Function),
      );
    });

    it("throws on failure", async () => {
      mockGh("", "not found", 1);

      await expect(client.closeIssue(999)).rejects.toThrow(
        "Failed to close issue",
      );
    });
  });

  describe("getIssueBody", () => {
    it("returns the issue body", async () => {
      mockGh("This is the issue body\n");

      const body = await client.getIssueBody(10);

      expect(body).toBe("This is the issue body");
      expect(childProcess.execFile).toHaveBeenCalledWith(
        "gh",
        [
          "issue",
          "view",
          "10",
          "--repo",
          "owner/repo",
          "--json",
          "body",
          "--jq",
          ".body",
        ],
        expect.any(Object),
        expect.any(Function),
      );
    });

    it("throws on failure", async () => {
      mockGh("", "not found", 1);

      await expect(client.getIssueBody(999)).rejects.toThrow(
        "Failed to get issue body",
      );
    });
  });
});
