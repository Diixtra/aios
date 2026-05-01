/**
 * code-pr agent postflight — open a PR for the branch pi delivered.
 *
 * Decision matrix:
 *   - status="ready"  -> regular PR (ready for review).
 *   - status="draft"  -> draft PR (work-in-progress; keeps reviewers
 *                          out of the inbox until the agent / a human
 *                          marks it ready).
 *   - status="error"  -> no PR; error surfaces to the caller so the
 *                          AgentTask can be marked Failed.
 *   - branch missing  -> hard error; pi must always return a branch
 *                          when status is ready/draft.
 *
 * The structural `Gh` interface accepts only `openPR` so this module
 * stays trivially testable without dragging in the gh-CLI wrapper.
 */
import type { PiResult } from "./run-pi.js";

export interface PostflightGh {
  openPR(spec: {
    repo: string;
    head: string;
    title: string;
    body: string;
    draft: boolean;
  }): Promise<{ url: string }>;
}

export interface PostflightOpts {
  result: PiResult;
  repo: string;
  issue: number;
  gh: PostflightGh;
}

export interface PostflightResult {
  prUrl: string;
}

export async function postflight(
  opts: PostflightOpts,
): Promise<PostflightResult> {
  const { result, repo, issue, gh } = opts;

  if (result.status === "error") {
    throw new Error(`pi returned error status: ${result.summary}`);
  }
  if (!result.branch) {
    throw new Error(
      `postflight: pi returned no branch (status=${result.status})`,
    );
  }

  const pr = await gh.openPR({
    repo,
    head: result.branch,
    title: `Closes #${issue}: ${result.summary}`,
    body: `Closes #${issue}\n\n${result.summary}`,
    draft: result.status === "draft",
  });
  return { prUrl: pr.url };
}
