import { execFile } from "node:child_process";
import type { CommandResult } from "./types.js";

/**
 * GitHub client wrapping the `gh` CLI for PR and issue lifecycle management.
 */
export class GitHubClient {
  private readonly repo: string;

  constructor(repo: string) {
    this.repo = repo;
  }

  /**
   * Execute a gh CLI command and return the result.
   */
  private exec(args: string[]): Promise<CommandResult> {
    return new Promise((resolve) => {
      execFile("gh", args, { maxBuffer: 5 * 1024 * 1024 }, (error, stdout, stderr) => {
        resolve({
          exitCode: error?.code !== undefined ? (error.code as number) : 0,
          stdout: stdout ?? "",
          stderr: stderr ?? "",
        });
      });
    });
  }

  /**
   * Create a pull request and return the PR URL.
   * @param title - PR title
   * @param body - PR description body
   * @returns The URL of the created PR
   */
  async createPR(title: string, body: string): Promise<string> {
    const result = await this.exec([
      "pr",
      "create",
      "--repo",
      this.repo,
      "--title",
      title,
      "--body",
      body,
    ]);

    if (result.exitCode !== 0) {
      throw new Error(`Failed to create PR: ${result.stderr}`);
    }

    return result.stdout.trim();
  }

  /**
   * Update a label on a GitHub issue.
   * @param issueNumber - The issue number
   * @param label - Label to add
   */
  async updateIssueLabel(issueNumber: number, label: string): Promise<void> {
    const result = await this.exec([
      "issue",
      "edit",
      String(issueNumber),
      "--repo",
      this.repo,
      "--add-label",
      label,
    ]);

    if (result.exitCode !== 0) {
      throw new Error(`Failed to update issue label: ${result.stderr}`);
    }
  }

  /**
   * Close a GitHub issue.
   * @param issueNumber - The issue number to close
   */
  async closeIssue(issueNumber: number): Promise<void> {
    const result = await this.exec([
      "issue",
      "close",
      String(issueNumber),
      "--repo",
      this.repo,
    ]);

    if (result.exitCode !== 0) {
      throw new Error(`Failed to close issue: ${result.stderr}`);
    }
  }

  /**
   * Get the body text of a GitHub issue.
   * @param issueNumber - The issue number
   * @returns The issue body text
   */
  async getIssueBody(issueNumber: number): Promise<string> {
    const result = await this.exec([
      "issue",
      "view",
      String(issueNumber),
      "--repo",
      this.repo,
      "--json",
      "body",
      "--jq",
      ".body",
    ]);

    if (result.exitCode !== 0) {
      throw new Error(`Failed to get issue body: ${result.stderr}`);
    }

    return result.stdout.trim();
  }
}
