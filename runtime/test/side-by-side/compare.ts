/**
 * Side-by-side comparison harness for Phase 1 acceptance.
 *
 * For each fixture issue, dispatches one AgentTask CR per engine
 * (claude-sdk + pi), polls each to completion, and writes a markdown
 * report comparing PR URLs and CI statuses.
 *
 * Usage:
 *   tsx runtime/test/side-by-side/compare.ts \
 *     --fixtures runtime/test/side-by-side/fixtures \
 *     --repo Diixtra/aios-test-fixtures \
 *     --namespace aios \
 *     [--engines claude-sdk,pi]
 *
 * Prerequisites:
 *   - kubectl context set to a cluster running the operator
 *   - GITHUB_TOKEN env var with read access to the test repo's issues + PRs
 *   - The fixture issues must already exist as real issues on the test repo
 *     (matched by frontmatter `slug`); see fixtures/README.md for how the
 *     mapping works
 */

import { promises as fs } from "node:fs";
import path from "node:path";
import { spawn } from "node:child_process";

interface Args {
  fixtures: string;
  repo: string;
  namespace: string;
  engines: string[];
  pollIntervalSec: number;
  timeoutMin: number;
  out: string;
}

interface FixtureFrontmatter {
  title: string;
  slug: string;
  difficulty: string;
  agentType: string;
}

interface FixtureFile {
  frontmatter: FixtureFrontmatter;
  body: string;
  path: string;
}

interface DispatchResult {
  fixture: string;
  engine: string;
  agentTaskName: string;
  finalPhase: string;
  prUrl?: string;
  failureReason?: string;
  startedAt?: string;
  completedAt?: string;
}

function parseArgs(argv: string[]): Args {
  const args: Record<string, string> = {};
  for (let i = 0; i < argv.length; i++) {
    if (argv[i].startsWith("--")) {
      args[argv[i].slice(2)] = argv[i + 1];
      i++;
    }
  }
  if (!args.fixtures || !args.repo) {
    throw new Error("required: --fixtures <dir> --repo <owner/repo>");
  }
  return {
    fixtures: args.fixtures,
    repo: args.repo,
    namespace: args.namespace ?? "aios",
    engines: (args.engines ?? "claude-sdk,pi").split(",").map((s) => s.trim()),
    pollIntervalSec: parseInt(args.pollIntervalSec ?? "30", 10),
    timeoutMin: parseInt(args.timeoutMin ?? "30", 10),
    out: args.out ?? path.join(args.fixtures, "..", "report.md"),
  };
}

async function readFixtures(dir: string): Promise<FixtureFile[]> {
  const entries = await fs.readdir(dir);
  const fixtures: FixtureFile[] = [];
  for (const e of entries.sort()) {
    if (!e.endsWith(".md")) continue;
    const full = path.join(dir, e);
    const text = await fs.readFile(full, "utf8");
    const fm = parseFrontmatter(text);
    if (!fm) {
      console.warn(`skipping ${e}: no frontmatter`);
      continue;
    }
    fixtures.push({ frontmatter: fm.frontmatter, body: fm.body, path: full });
  }
  return fixtures;
}

export function parseFrontmatter(
  text: string,
): { frontmatter: FixtureFrontmatter; body: string } | null {
  const match = text.match(/^---\n([\s\S]*?)\n---\n([\s\S]*)$/);
  if (!match) return null;
  const yaml = match[1];
  const body = match[2];
  const fm: Record<string, string> = {};
  for (const line of yaml.split("\n")) {
    const m = line.match(/^(\w+):\s*(.+)$/);
    if (!m) continue;
    fm[m[1]] = m[2].replace(/^["']|["']$/g, "");
  }
  return {
    frontmatter: {
      title: fm.title ?? "",
      slug: fm.slug ?? "",
      difficulty: fm.difficulty ?? "",
      agentType: fm.agentType ?? "code-pr",
    },
    body,
  };
}

function sh(cmd: string, args: string[], opts: { stdin?: string } = {}): Promise<{ code: number; stdout: string; stderr: string }> {
  return new Promise((resolve) => {
    const proc = spawn(cmd, args, { stdio: ["pipe", "pipe", "pipe"] });
    let stdout = "";
    let stderr = "";
    proc.stdout.on("data", (b) => (stdout += b.toString()));
    proc.stderr.on("data", (b) => (stderr += b.toString()));
    proc.on("close", (code) => resolve({ code: code ?? 1, stdout, stderr }));
    if (opts.stdin) {
      proc.stdin.write(opts.stdin);
    }
    proc.stdin.end();
  });
}

async function findIssueNumber(repo: string, slug: string): Promise<number> {
  const r = await sh("gh", [
    "issue",
    "list",
    "--repo",
    repo,
    "--state",
    "open",
    "--search",
    `${slug} in:title,body`,
    "--json",
    "number,title",
    "--limit",
    "5",
  ]);
  if (r.code !== 0) {
    throw new Error(`gh issue list failed: ${r.stderr}`);
  }
  const items = JSON.parse(r.stdout) as { number: number; title: string }[];
  if (items.length === 0) {
    throw new Error(`no open issue matching slug=${slug} in ${repo}`);
  }
  return items[0].number;
}

async function dispatchAgentTask(
  args: Args,
  fixture: FixtureFile,
  engine: string,
  issueNumber: number,
): Promise<string> {
  const taskName = `compare-${fixture.frontmatter.slug}-${engine}-${Date.now()}`.toLowerCase();
  const yaml = `apiVersion: aios.kazie.co.uk/v1alpha1
kind: AgentTask
metadata:
  name: ${taskName}
  namespace: ${args.namespace}
  labels:
    aios.kazie.co.uk/comparison: "true"
    aios.kazie.co.uk/engine: ${engine}
    aios.kazie.co.uk/fixture: ${fixture.frontmatter.slug}
spec:
  source:
    type: github-issue
    repo: ${args.repo}
    issueNumber: ${issueNumber}
  prompt: |
    ${fixture.body.split("\n").join("\n    ")}
  agentType: ${fixture.frontmatter.agentType}
  agentConfig: ${engine === "pi" ? "code-pr-pi" : "code-pr"}
  toolPolicy: code-pr
  priority: normal
  timeout: ${args.timeoutMin}m
`;
  const r = await sh("kubectl", ["apply", "-f", "-"], { stdin: yaml });
  if (r.code !== 0) {
    throw new Error(`kubectl apply failed: ${r.stderr}`);
  }
  return taskName;
}

async function pollUntilTerminal(
  args: Args,
  taskName: string,
): Promise<{ phase: string; prUrl?: string; failureReason?: string; startedAt?: string; completedAt?: string }> {
  const deadline = Date.now() + args.timeoutMin * 60_000;
  while (Date.now() < deadline) {
    const r = await sh("kubectl", [
      "-n",
      args.namespace,
      "get",
      "agenttask",
      taskName,
      "-o",
      "json",
    ]);
    if (r.code !== 0) {
      throw new Error(`kubectl get failed: ${r.stderr}`);
    }
    const obj = JSON.parse(r.stdout) as { status?: { phase?: string; prUrl?: string; failureReason?: string; startedAt?: string; completedAt?: string } };
    const phase = obj.status?.phase ?? "Pending";
    if (phase === "Completed" || phase === "Failed") {
      return {
        phase,
        prUrl: obj.status?.prUrl,
        failureReason: obj.status?.failureReason,
        startedAt: obj.status?.startedAt,
        completedAt: obj.status?.completedAt,
      };
    }
    await new Promise((res) => setTimeout(res, args.pollIntervalSec * 1000));
  }
  return { phase: "TimedOut" };
}

async function checkPrCi(repo: string, prUrl: string): Promise<{ status: string; checks?: { name: string; conclusion: string }[] }> {
  if (!prUrl) return { status: "no-pr" };
  const m = prUrl.match(/\/pull\/(\d+)/);
  if (!m) return { status: "unparseable-url" };
  const number = m[1];
  const r = await sh("gh", [
    "pr",
    "view",
    number,
    "--repo",
    repo,
    "--json",
    "statusCheckRollup,state,isDraft",
  ]);
  if (r.code !== 0) return { status: "fetch-failed" };
  const obj = JSON.parse(r.stdout) as {
    statusCheckRollup?: { name: string; conclusion: string }[];
    state: string;
    isDraft: boolean;
  };
  const checks = (obj.statusCheckRollup ?? []).map((c) => ({ name: c.name, conclusion: c.conclusion ?? "PENDING" }));
  const overall = checks.length === 0
    ? "no-checks"
    : checks.every((c) => c.conclusion === "SUCCESS")
      ? "passing"
      : checks.some((c) => c.conclusion === "FAILURE")
        ? "failing"
        : "pending";
  return { status: `${obj.isDraft ? "draft/" : ""}${obj.state.toLowerCase()}/${overall}`, checks };
}

function renderReport(args: Args, results: DispatchResult[], ciByPr: Map<string, { status: string }>): string {
  const lines: string[] = [];
  lines.push(`# code-pr side-by-side comparison report`);
  lines.push("");
  lines.push(`- Generated: ${new Date().toISOString()}`);
  lines.push(`- Repo: ${args.repo}`);
  lines.push(`- Engines: ${args.engines.join(" vs ")}`);
  lines.push(`- Fixtures: ${args.fixtures}`);
  lines.push("");
  lines.push("## Results");
  lines.push("");
  lines.push("| Fixture | Engine | Phase | PR | CI |");
  lines.push("|---|---|---|---|---|");
  for (const r of results) {
    const ci = r.prUrl ? (ciByPr.get(r.prUrl)?.status ?? "?") : "—";
    const pr = r.prUrl ? `[link](${r.prUrl})` : (r.failureReason ?? "—");
    lines.push(`| \`${r.fixture}\` | ${r.engine} | ${r.finalPhase} | ${pr} | ${ci} |`);
  }
  lines.push("");
  lines.push("## Per-fixture verdict");
  lines.push("");
  // Group by fixture, compare engines pairwise.
  const byFixture = new Map<string, DispatchResult[]>();
  for (const r of results) {
    const list = byFixture.get(r.fixture) ?? [];
    list.push(r);
    byFixture.set(r.fixture, list);
  }
  for (const [fixture, runs] of byFixture) {
    lines.push(`### ${fixture}`);
    const allCompleted = runs.every((r) => r.finalPhase === "Completed");
    const allHavePr = runs.every((r) => !!r.prUrl);
    const allCiClean = runs.every((r) => {
      const status = r.prUrl ? ciByPr.get(r.prUrl)?.status : "—";
      return status === "passing" || status?.endsWith("/passing");
    });
    lines.push(`- Both engines completed: ${allCompleted ? "✅" : "❌"}`);
    lines.push(`- Both opened a PR: ${allHavePr ? "✅" : "❌"}`);
    lines.push(`- Both CI green: ${allCiClean ? "✅" : "❌"}`);
    lines.push("");
  }
  lines.push("## Manual review TODO");
  lines.push("");
  lines.push("Open both PRs side-by-side and judge code quality (clarity, idiomatic style, test additions). Score each on a 1-5 scale and append below.");
  lines.push("");
  for (const fixture of byFixture.keys()) {
    lines.push(`- [ ] **${fixture}** — claude-sdk: ?/5, pi: ?/5, notes:`);
  }
  return lines.join("\n") + "\n";
}

async function main(): Promise<void> {
  const args = parseArgs(process.argv.slice(2));
  console.log(`reading fixtures from ${args.fixtures}`);
  const fixtures = await readFixtures(args.fixtures);
  console.log(`found ${fixtures.length} fixtures`);

  const dispatchedRuns: { fixture: FixtureFile; engine: string; taskName: string }[] = [];
  for (const fixture of fixtures) {
    let issueNumber: number;
    try {
      issueNumber = await findIssueNumber(args.repo, fixture.frontmatter.slug);
    } catch (err) {
      console.error(`skipping ${fixture.frontmatter.slug}: ${(err as Error).message}`);
      continue;
    }
    for (const engine of args.engines) {
      console.log(`dispatching ${fixture.frontmatter.slug} -> ${engine} -> issue #${issueNumber}`);
      const taskName = await dispatchAgentTask(args, fixture, engine, issueNumber);
      dispatchedRuns.push({ fixture, engine, taskName });
    }
  }

  console.log(`\npolling ${dispatchedRuns.length} AgentTasks (interval=${args.pollIntervalSec}s, timeout=${args.timeoutMin}m)…\n`);
  const results: DispatchResult[] = [];
  for (const run of dispatchedRuns) {
    const status = await pollUntilTerminal(args, run.taskName);
    console.log(`  ${run.fixture.frontmatter.slug}/${run.engine}: ${status.phase}`);
    results.push({
      fixture: run.fixture.frontmatter.slug,
      engine: run.engine,
      agentTaskName: run.taskName,
      finalPhase: status.phase,
      prUrl: status.prUrl,
      failureReason: status.failureReason,
      startedAt: status.startedAt,
      completedAt: status.completedAt,
    });
  }

  console.log(`\nfetching CI status for ${results.filter((r) => r.prUrl).length} PRs…`);
  const ciByPr = new Map<string, { status: string }>();
  for (const r of results) {
    if (!r.prUrl) continue;
    const ci = await checkPrCi(args.repo, r.prUrl);
    ciByPr.set(r.prUrl, { status: ci.status });
  }

  const report = renderReport(args, results, ciByPr);
  await fs.writeFile(args.out, report);
  console.log(`\nreport written to ${args.out}`);
}

if (import.meta.url === `file://${process.argv[1]}`) {
  main().catch((err) => {
    console.error(err);
    process.exit(1);
  });
}
