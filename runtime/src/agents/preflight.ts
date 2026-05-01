/**
 * code-pr agent preflight — clone the target repo and fetch the issue body
 * before the pi-driven implementation phase begins.
 *
 * The structural `GhClient` interface is intentionally minimal: it accepts
 * the two GitHub-side actions preflight needs and nothing else. Production
 * code passes a real `gh`-CLI-backed client; tests pass `vi.fn()` fakes.
 */

export interface GhClient {
  /** Clone the named repository (slug `owner/name`) and return the local path. */
  cloneRepo(slug: string): Promise<string>;
  /** Fetch the title and body of the named issue. */
  getIssue(
    repo: string,
    number: number,
  ): Promise<{ title: string; body: string }>;
}

export interface PreflightResult {
  repoDir: string;
  issueTitle: string;
  issueBody: string;
}

/**
 * Run preflight: clone the repo, fetch the issue, return the handoff.
 *
 * Errors propagate unchanged. Failing fast here means we never invoke pi
 * with an incomplete bundle.
 */
export async function preflight(opts: {
  repo: string;
  issue: number;
  gh: GhClient;
}): Promise<PreflightResult> {
  const repoDir = await opts.gh.cloneRepo(opts.repo);
  const issue = await opts.gh.getIssue(opts.repo, opts.issue);
  return {
    repoDir,
    issueTitle: issue.title,
    issueBody: issue.body,
  };
}
