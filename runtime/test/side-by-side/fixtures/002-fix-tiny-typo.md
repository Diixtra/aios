---
title: "Fix typo: 'recieve' → 'receive' across the codebase"
slug: 002-fix-tiny-typo
difficulty: trivial
agentType: code-pr
---

Search the repo (excluding `node_modules/`, `vendor/`, `dist/`, `.git/`) for the misspelling **`recieve`** (and casings like `Recieve`, `RECIEVE`). Replace each with the correct **`receive`** preserving the original casing.

Constraints:

- Don't touch fixture files inside `runtime/test/side-by-side/fixtures/` even if they happen to contain the typo (this is a self-referential trap; ignore it).
- Don't modify lockfiles (`*-lock.json`, `go.sum`, `Cargo.lock`).
- If a code identifier (variable / function / type) contains the misspelling, rename it across the codebase using your IDE's safe-rename equivalent — but only do this if the rename is *purely* mechanical. If the identifier is part of a public API, open the PR as a draft and flag it.

**Acceptance:** No instances of `recieve`/`Recieve`/`RECIEVE` remain (verify with `rg -i 'recieve' --glob '!**/node_modules/**' --glob '!**/dist/**' --glob '!**/vendor/**'`); tests pass; PR description lists the file count and identifier count.
