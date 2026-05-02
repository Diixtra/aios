---
title: "Add vitest coverage for runtime/src/config.ts edge cases"
slug: 005-add-vitest-test
difficulty: small
agentType: code-pr
---

`runtime/src/config.ts` already has a `config.test.ts`, but it covers only the happy path. Extend the test file with cases for the documented edge cases the implementation handles.

Concretely:

- Read `runtime/src/config.ts` and identify each branch that handles a missing-or-invalid input (env var unset, empty string, unparseable number/duration, etc.).
- For each branch that lacks a test, add one. Use the existing test file's style (vitest `describe`/`it`, no extra imports beyond what's already used).
- Don't refactor `config.ts` itself — this issue is *test additions only*. If you find a bug in `config.ts`, open a separate issue and note it in the PR description; don't fix it here.
- Run `npm test` and confirm the new tests pass.

**Acceptance:** `runtime/src/config.test.ts` has new `it(...)` blocks for each previously-untested edge branch; `npm test` from `runtime/` is green; no changes to `runtime/src/config.ts`.
