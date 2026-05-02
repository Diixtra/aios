---
title: "Bump operator's controller-runtime dependency to the latest minor"
slug: 004-bump-go-mod
difficulty: trivial-config
agentType: code-pr
---

The `operator` Go module pins `sigs.k8s.io/controller-runtime` to a specific minor version. Bump it to the latest available minor in the same major (i.e. don't cross a major boundary).

Steps:

- From `operator/`, run `go list -m -u sigs.k8s.io/controller-runtime` to see the current and latest versions.
- Run `go get sigs.k8s.io/controller-runtime@latest`.
- Run `go mod tidy`.
- Run `make test` (or `go test ./...` if no Makefile target exists).
- If tests pass, commit `operator/go.mod` + `operator/go.sum`.
- If tests fail, capture the failure log in the PR description and **open as draft** — don't try to fix unrelated breakages.

**Acceptance:** `operator/go.mod` shows the new minor version; `go.sum` updated; tests green or PR is draft with the failure attached.
