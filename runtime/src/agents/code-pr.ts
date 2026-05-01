/**
 * code-pr agent entrypoint.
 *
 * The operator launches this binary inside a per-AgentTask Job whenever an
 * AgentConfig with `engine: pi` is reconciled. End-to-end flow:
 *
 *   1. acquireLease — block until the auth-broker grants exclusive
 *      ownership of the ChatGPT-subscription credential bundle.
 *   2. mkdtemp + downloadBundle — drop the bundle into a per-Job pi state
 *      directory so two Jobs can never share a mutating auth.json
 *      (Spike A5).
 *   3. preflight — clone the target repo + fetch the issue body.
 *   4. runPi — drive pi via the SDK with sandbox + slack-thread + MCP
 *      extensions and the deliver_result synthesis tool.
 *   5. uploadPostRunBundle — push the rotated access-token back to the
 *      broker (best-effort; non-fatal on failure).
 *   6. postflight — open a draft / real PR per the deliver_result status.
 *   7. releaseLease + cleanup — always-runs `finally`.
 */
import { promises as fs } from "node:fs";
import path from "node:path";
import * as piSdk from "@mariozechner/pi-coding-agent";

import {
  acquireLease,
  downloadBundle,
  releaseLease,
  uploadPostRunBundle,
} from "../auth-broker.js";
import { discoverFabricSkills } from "../../pi-extensions/fabric-skill/index.js";
import { GitHubClient } from "../github.js";
import { preflight, type GhClient } from "./preflight.js";
import { postflight, type PostflightGh } from "./postflight.js";
import { runPi, type RunPiSdkLike } from "./run-pi.js";

export { preflight } from "./preflight.js";
export type { GhClient, PreflightResult } from "./preflight.js";
export { postflight } from "./postflight.js";

function mustEnv(name: string): string {
  const value = process.env[name];
  if (!value) throw new Error(`missing env ${name}`);
  return value;
}

/**
 * Adapter that maps the existing `GitHubClient` (gh-CLI wrapper) onto the
 * structural `GhClient` interface preflight expects.
 *
 * `cloneRepo` shells out to `gh repo clone <slug>` into a fresh tempdir and
 * returns the local path. `getIssue` uses gh's existing `getIssueBody`
 * helper plus a JSON title fetch.
 */
class GhCli implements GhClient, PostflightGh {
  async cloneRepo(slug: string): Promise<string> {
    const dir = await fs.mkdtemp(path.join("/tmp", "aios-repo-"));
    const { execFile } = await import("node:child_process");
    await new Promise<void>((resolve, reject) => {
      execFile(
        "gh",
        ["repo", "clone", slug, dir, "--", "--depth", "1"],
        (err) => (err ? reject(err) : resolve()),
      );
    });
    return dir;
  }

  async getIssue(
    repo: string,
    number: number,
  ): Promise<{ title: string; body: string }> {
    const client = new GitHubClient(repo);
    const body = await client.getIssueBody(number);
    // gh's title fetch isn't a method on GitHubClient yet; shell out directly.
    const { execFile } = await import("node:child_process");
    const title = await new Promise<string>((resolve, reject) => {
      execFile(
        "gh",
        [
          "issue",
          "view",
          String(number),
          "--repo",
          repo,
          "--json",
          "title",
          "--jq",
          ".title",
        ],
        (err, stdout) => (err ? reject(err) : resolve((stdout ?? "").trim())),
      );
    });
    return { title, body };
  }

  /** Open a PR via gh CLI. Maps the structural spec onto `gh pr create`. */
  async openPR(spec: {
    repo: string;
    head: string;
    title: string;
    body: string;
    draft: boolean;
  }): Promise<{ url: string }> {
    const args = [
      "pr",
      "create",
      "--repo",
      spec.repo,
      "--head",
      spec.head,
      "--title",
      spec.title,
      "--body",
      spec.body,
    ];
    if (spec.draft) args.push("--draft");
    const { execFile } = await import("node:child_process");
    const url = await new Promise<string>((resolve, reject) => {
      execFile("gh", args, (err, stdout) => {
        if (err) {
          reject(err);
          return;
        }
        resolve((stdout ?? "").trim());
      });
    });
    return { url };
  }
}

/**
 * Entrypoint. Reads required env (AIOS_REPO, AIOS_ISSUE_NUMBER,
 * AIOS_PI_MODEL, etc.), runs the lease-bracketed pipeline, and exits.
 *
 * Postflight (PR creation) lands in C11 — the current build logs the
 * pi result and exits 0/1 based on `status`.
 */
export async function main(): Promise<void> {
  const repo = mustEnv("AIOS_REPO");
  const issue = parseInt(mustEnv("AIOS_ISSUE_NUMBER"), 10);
  const lease = await acquireLease("code-pr");

  const piDir = await fs.mkdtemp("/tmp/pi-state-");
  try {
    // Per-Job bundle copy (Spike A5).
    const bundle = await downloadBundle();
    await fs.writeFile(path.join(piDir, "auth.json"), bundle, { mode: 0o600 });

    const gh = new GhCli();
    const pf = await preflight({ repo, issue, gh });
    const piModel = mustEnv("AIOS_PI_MODEL");
    const systemPrompt = await fs.readFile("/agent-config/SYSTEM.md", "utf8");
    const skills = await discoverFabricSkills("/fabric-patterns", [
      "extract_requirements",
      "write_pull_request",
      "find_hidden_bugs",
    ]);

    const result = await runPi({
      sdk: piSdk as unknown as RunPiSdkLike,
      model: piModel,
      systemPrompt,
      skills,
      piDir,
      prompt: `Implement issue ${repo}#${issue}: ${pf.issueTitle}\n\n${pf.issueBody}`,
      sandboxOpts: {
        allowed: ["git", "npm", "go", "pytest", "rg", "cat", "ls"],
      },
      mcpServers: [
        { name: "aios-search", url: process.env.AIOS_SEARCH_URL ?? "" },
        { name: "memory", url: process.env.MEMORY_MCP_URL ?? "" },
      ].filter((s) => s.url),
    });

    // Post-run upload-back (Spike A4/A6: access token rotates).
    try {
      const updated = await fs.readFile(path.join(piDir, "auth.json"));
      await uploadPostRunBundle(updated);
    } catch (err) {
      console.warn("post-run bundle upload failed (non-fatal):", err);
    }

    const pr = await postflight({ result, repo, issue, gh });
    console.log(JSON.stringify({ result, prUrl: pr.prUrl }));
  } finally {
    await fs.rm(piDir, { recursive: true, force: true });
    await releaseLease(lease.id);
  }
}

// Auto-execute when invoked directly. We guard so the module can also be
// imported safely from tests / other entrypoints.
if (process.argv[1] && process.argv[1].endsWith("code-pr.js")) {
  main().catch((e) => {
    console.error(e);
    process.exit(1);
  });
}
