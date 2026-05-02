# Side-by-side comparison harness (Phase 1 acceptance gate)

Dispatches each fixture issue through both `claude-sdk` and `pi` engines via real `AgentTask` CRs, polls them to completion, and produces a comparison report. Used to validate that the pi engine produces equal-or-better PRs before Phase 1 closes.

## Prerequisites

1. **A test repo** — not the main `aios` repo. The harness opens real PRs; use a sandbox like `Diixtra/aios-test-fixtures`. You'll need write access for the operator's GitHub App.
2. **The fixture issues filed as real GitHub issues** in that test repo, with titles or bodies containing the slug from each fixture's frontmatter (e.g. `001-add-readme-section`). The harness greps for the slug to find the matching issue number.
3. **The operator + auth-broker deployed** in the cluster, with both `code-pr` (claude-sdk) and `code-pr-pi` AgentConfigs available, and the auth-broker bootstrapped.
4. `kubectl` configured against that cluster, and `gh` authenticated with read access to the test repo.

## Run

```bash
tsx runtime/test/side-by-side/compare.ts \
  --fixtures runtime/test/side-by-side/fixtures \
  --repo Diixtra/aios-test-fixtures \
  --namespace aios \
  --engines claude-sdk,pi \
  --timeoutMin 30 \
  --out runtime/test/side-by-side/report.md
```

Output: `report.md` with one row per fixture × engine, plus a manual-review checklist for code-quality scoring.

## Fixtures

| Slug | Difficulty | What it tests |
|---|---|---|
| `001-add-readme-section` | easy | Markdown editing, structure preservation |
| `002-fix-tiny-typo` | trivial | Repo-wide search/replace with exclusions |
| `003-add-go-env-var` | small | Go logic + matching unit test |
| `004-bump-go-mod` | trivial-config | Mechanical dep bump + test gate |
| `005-add-vitest-test` | small | TypeScript test additions, no impl changes |

Add or replace fixtures by dropping more `NNN-<slug>.md` files into `fixtures/` with the same frontmatter shape. The harness reads them in lexical order.

## Acceptance criteria (C14)

After running:

1. Every fixture's two PRs (`claude-sdk` + `pi`) opened and CI passing — recorded as ✅/❌ in the auto-generated `## Per-fixture verdict` table.
2. Manual review: open both PRs side-by-side, score each on a 1-5 scale (clarity, idiomatic style, test additions), fill in the checklist at the bottom of the report.
3. Decision rule: if pi engine ≥ claude-sdk on ≥4/5 issues (CI + manual score combined), declare Phase 1 success. Otherwise file an issue documenting the gap and iterate within Phase 1 before moving on.

The report file is committed alongside the comparison run as `runtime/test/side-by-side/2026-XX-XX-results.md`.
