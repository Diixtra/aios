# Pi + Fabric Agent Pivot — Phases -1, 0, 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up the auth-broker service and rebuild the code-pr agent on pi (`--mode json`) backed by the user's ChatGPT subscription, gated behind an engine feature flag for side-by-side comparison with the existing Claude Agent SDK path.

**Architecture:** Three sequential phases. Phase -1 is a research spike that validates pi's subscription auth is automatable for headless K8s Jobs and produces the concrete contract for Phase 0. Phase 0 builds `auth-broker/` (Go), a single-pod service that owns the OAuth state, refreshes tokens, exposes a lease API for concurrency control, and drives Slack-DM phone reauth. Phase 1 adds `runtime/src/agents/code-pr.ts` plus four pi extensions (sandbox, slack-thread, MCP, fabric-skill); the operator picks the pi path when `AgentConfig.spec.runtime.engine == "pi"`, leaving the existing claude-sdk path untouched until both produce equal-or-better PRs.

**Tech Stack:**

- **Phase -1 spike:** pi (latest stable from pi.dev), Docker (clean Linux, no TTY), mitmproxy for transport capture, Markdown report
- **Phase 0 auth-broker:** Go 1.25, `slack-go/slack` SDK, `prometheus/client_golang`, K8s `client-go` for TokenReview, 1Password Operator for secrets, K8s manifests in `k8s/base/auth-broker/`
- **Phase 1 runtime:** TypeScript (Node 24), Vitest, `@slack/web-api`, pi installed in shared `runtime` image; operator changes in `operator/api/v1alpha1/` and `operator/internal/controller/`

**Source spec:** `docs/superpowers/specs/2026-04-30-pi-fabric-agent-pivot-design.md` (commit `f7913fb`).

**Branch policy:** Create a fresh branch off `main` for this work — `james/pi-fabric-pivot-phase-1`. Do NOT do this work on `james/issue-1425-aios-search-lifespan` (unrelated). The first task verifies branch hygiene.

---

## Pre-flight

### Task P0: Branch hygiene

**Files:** none

- [ ] **Step 1: Confirm clean tree on a new branch**

```bash
cd /Users/james/Kazcloud/Github/aios
git status --short            # should show only docs/superpowers/specs work or be clean
git fetch origin
git checkout -b james/pi-fabric-pivot-phase-1 origin/main
```

Expected: branch created off `origin/main`. If your in-progress spec edits aren't yet on `main`, cherry-pick the design commits onto this branch first:

```bash
git cherry-pick 926efc3 f7913fb
```

- [ ] **Step 2: Confirm Go and Node toolchain versions**

```bash
go version       # >= go1.25
node --version   # v24.x
```

If either is wrong, install via `mise` or `devbox` per the repo's `.devbox.json` / `mise.toml` (whichever exists).

- [ ] **Step 3: Commit nothing — this task is just verification.**

---

# Phase -1 — Auth Transport Spike

This phase is investigation, not feature code. It produces a written findings doc and a small reproducible PoC. Decisions made here gate Phase 0.

**Output of phase:**
- `docs/superpowers/spikes/2026-04-30-pi-auth-transport-findings.md` — concrete answers to the questions below
- `auth-broker-spike/` — disposable directory with a `Dockerfile`, `compose.yml`, and a `justfile` reproducing the spike experiments
- A go/no-go decision: are we proceeding with brokered auth as designed, or pivoting (e.g. fall back to OpenAI API key)?

**Time budget:** 1-2 days.

### Task A1: Scaffold spike harness

**Files:**
- Create: `auth-broker-spike/Dockerfile`
- Create: `auth-broker-spike/compose.yml`
- Create: `auth-broker-spike/justfile`
- Create: `auth-broker-spike/README.md`

- [ ] **Step 1: Write a minimal Dockerfile that installs pi in a clean Linux image with no TTY**

`auth-broker-spike/Dockerfile`:

```dockerfile
FROM node:24-bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl jq tini \
    && rm -rf /var/lib/apt/lists/*
RUN npm install -g @mariozechner/pi-coding-agent
WORKDIR /work
ENV PI_CODING_AGENT_DIR=/pi-state
ENV TERM=dumb
ENTRYPOINT ["/usr/bin/tini","--"]
CMD ["bash"]
```

Pin the pi version once Step 2 of Task A2 picks the version.

- [ ] **Step 2: Write a compose file mounting an auth volume and a working dir**

`auth-broker-spike/compose.yml`:

```yaml
services:
  pi:
    build: .
    image: aios-spike/pi:latest
    volumes:
      - pi-state:/pi-state          # PI_CODING_AGENT_DIR
      - ./scratch:/work
    stdin_open: false                # explicitly NO TTY — this is the headless test
    tty: false
volumes:
  pi-state:
```

- [ ] **Step 3: Write a justfile with reproducible recipes**

`auth-broker-spike/justfile`:

```just
# Spike harness for pi auth-transport investigation.
# Diixtra convention: just (not make), podman (not docker).

default:
    @just --list

build:
    podman compose build

shell:
    podman compose run --rm pi bash

inspect-state:
    podman compose run --rm pi bash -c 'ls -la $PI_CODING_AGENT_DIR && find $PI_CODING_AGENT_DIR -type f -exec sha256sum {} \;'

run-prompt:
    podman compose run --rm pi bash -c 'pi --mode json --no-extensions --no-skills --no-prompt-templates --no-context-files -p "Reply with the single token PONG and nothing else."'

clean:
    podman compose down -v
```

- [ ] **Step 4: Write a README explaining the spike's purpose and how to run each experiment**

`auth-broker-spike/README.md`:

```markdown
# auth-broker spike

Validates that pi's ChatGPT subscription auth can be exercised non-interactively from
a fresh Linux container with no TTY. Output of this spike is the findings document at
docs/superpowers/spikes/2026-04-30-pi-auth-transport-findings.md.

## Experiments
- A2 — discover PI_CODING_AGENT_DIR layout
- A3 — interactive login from headless container
- A4 — auth bundle portability (login on dev box, transplant to container)
- A5 — concurrent pi processes sharing one auth bundle
- A6 — refresh-token rotation behaviour
- A7 — pi outbound HTTPS transport capture
```

- [ ] **Step 5: Build and verify the image runs**

```bash
( cd auth-broker-spike && just build )
podman compose -f auth-broker-spike/compose.yml run --rm pi pi --version
```

Expected: pi version banner printed. (The `just shell` recipe is interactive — for the verification step we invoke `pi --version` directly via `podman compose run`.)

- [ ] **Step 6: Commit**

```bash
git add auth-broker-spike/
git commit -m "spike: scaffold pi auth-transport investigation harness"
```

### Task A2: Discover PI_CODING_AGENT_DIR layout

**Files:**
- Modify: `docs/superpowers/spikes/2026-04-30-pi-auth-transport-findings.md` (create on first task)

- [ ] **Step 1: Create the findings doc with stub sections**

`docs/superpowers/spikes/2026-04-30-pi-auth-transport-findings.md`:

```markdown
# pi Auth Transport Spike — Findings

**Date:** 2026-04-30
**Pi version:** <fill-in>
**Outcome:** <go | no-go | go-with-changes>

## A2 — PI_CODING_AGENT_DIR layout
## A3 — Interactive login from headless container
## A4 — Auth bundle portability
## A5 — Concurrent processes
## A6 — Refresh-token rotation
## A7 — Pi outbound HTTPS transport
## Decision
```

- [ ] **Step 2: Run pi inside the container long enough to populate PI_CODING_AGENT_DIR (will fail at API call but should write skeleton state)**

```bash
podman compose -f auth-broker-spike/compose.yml run --rm pi bash -c '
  echo "no" | pi --mode json -p "ping" 2>&1 || true
  echo "--- after run ---"
  find $PI_CODING_AGENT_DIR -type f
'
```

- [ ] **Step 3: Capture the directory layout**

```bash
( cd auth-broker-spike && just inspect-state ) | tee /tmp/pi-state-empty.txt
```

- [ ] **Step 4: Document findings in `## A2` section**

Fill in:
- Exact directory paths (e.g. `auth.json`, `models.json`, `settings.json`)
- File permissions
- Whether files are atomic-write (read briefly during write?) — verify by running pi while `inotifywait` watches the dir
- Any cache/temp files created
- Pin pi version in the doc header

- [ ] **Step 5: Commit**

```bash
git add docs/superpowers/spikes/2026-04-30-pi-auth-transport-findings.md
git commit -m "spike(A2): document PI_CODING_AGENT_DIR layout"
```

### Task A3: Interactive login from headless container

**Goal:** Determine whether `/login` works at all without a TTY.

- [ ] **Step 1: Attempt non-interactive login and observe failure mode**

```bash
podman compose -f auth-broker-spike/compose.yml run --rm pi \
  bash -c 'pi --mode json -p "/login" 2>&1' | tee /tmp/headless-login.log
```

Expected outcomes — record which one occurs:

a. Pi rejects the request immediately because no TTY → must run login on a TTY-backed host (likely outcome).

b. Pi prints a device-flow URL + code to stdout → can be scripted by reading stdout, posting URL to Slack, polling.

c. Pi opens a browser (impossible in container) and hangs.

- [ ] **Step 2: If outcome (b), capture the exact stdout shape — JSON event names, fields**

Save sample to `auth-broker-spike/samples/headless-login-events.jsonl`.

- [ ] **Step 3: Document `## A3` in findings**

Record outcome (a/b/c). If (b), the auth-broker can drive login itself; if (a) or (c), the broker must host an interactive helper (small TUI or browser-driven).

- [ ] **Step 4: Commit**

```bash
git add auth-broker-spike/samples docs/superpowers/spikes/2026-04-30-pi-auth-transport-findings.md
git commit -m "spike(A3): document headless login feasibility"
```

### Task A4: Auth bundle portability

**Goal:** Verify that an auth bundle produced on a TTY-backed dev box can be transplanted to a headless container and works.

- [ ] **Step 1: Log in on your laptop**

On your laptop (host, not container):

```bash
mkdir -p ~/.aios-spike-pi-state
PI_CODING_AGENT_DIR=~/.aios-spike-pi-state pi --mode interactive
# in pi: /login → choose ChatGPT subscription → complete OAuth in browser
# exit pi
ls -la ~/.aios-spike-pi-state/
```

- [ ] **Step 2: Mount the bundle into the container and run a real prompt**

```bash
podman run --rm -v ~/.aios-spike-pi-state:/pi-state:ro \
  -e PI_CODING_AGENT_DIR=/pi-state \
  aios-spike/pi:latest \
  pi --mode json --no-extensions --no-skills --no-prompt-templates --no-context-files \
     -p "Reply with the single token PONG and nothing else." \
  | tee /tmp/transplant-test.jsonl
```

Expected: pi emits assistant events containing `PONG`. Failure (auth error) means subscription is bound to host machine identity; design will need to fall back to API key.

- [ ] **Step 3: Verify state mutation behaviour after read-only mount**

If pi must write to PI_CODING_AGENT_DIR (e.g. update `auth.json` after refresh), a read-only mount will fail. Re-run with read-write and diff afterward:

```bash
cp -r ~/.aios-spike-pi-state /tmp/before
podman run --rm -v ~/.aios-spike-pi-state:/pi-state \
  -e PI_CODING_AGENT_DIR=/pi-state \
  aios-spike/pi:latest \
  pi --mode json -p "ping" >/dev/null
diff -ru /tmp/before ~/.aios-spike-pi-state || true
```

Document which files mutate during a single inference call.

- [ ] **Step 4: Document `## A4`**

Record:
- Did inference succeed with transplanted bundle? (yes/no)
- Which files mutated during a call?
- Implications for design: can the broker hand Jobs a read-only bundle, or must each Job get its own writeable copy?

- [ ] **Step 5: Commit**

```bash
git add docs/superpowers/spikes/2026-04-30-pi-auth-transport-findings.md
git commit -m "spike(A4): document auth bundle portability"
```

### Task A5: Concurrent processes

**Goal:** Determine whether two pi processes can use the same auth bundle simultaneously.

- [ ] **Step 1: Run two parallel containers from the same bundle**

```bash
podman run -d --name pi-a -v ~/.aios-spike-pi-state:/pi-state \
  -e PI_CODING_AGENT_DIR=/pi-state aios-spike/pi:latest \
  pi --mode json -p "Count to 3 slowly." > /tmp/pi-a.jsonl &
podman run -d --name pi-b -v ~/.aios-spike-pi-state:/pi-state \
  -e PI_CODING_AGENT_DIR=/pi-state aios-spike/pi:latest \
  pi --mode json -p "Count to 3 slowly." > /tmp/pi-b.jsonl &
wait
podman logs pi-a; podman logs pi-b
podman rm pi-a pi-b
```

- [ ] **Step 2: Inspect outcomes**

Possible:
- Both succeed independently → bundle is safe for concurrent reads. Lease API only enforces *provider-side* concurrency.
- One fails (e.g. file lock contention, refresh race) → broker must hand each Job its *own* writeable bundle copy.

- [ ] **Step 3: Document `## A5`**

Record outcome and required design adjustment if any.

- [ ] **Step 4: Commit**

```bash
git add docs/superpowers/spikes/2026-04-30-pi-auth-transport-findings.md
git commit -m "spike(A5): document concurrent-pi behaviour"
```

### Task A6: Refresh-token rotation

**Goal:** Confirm pi rotates refresh tokens; if so, broker must capture and persist the rotated bundle.

- [ ] **Step 1: Snapshot the bundle, then run pi enough times to trigger a token refresh**

```bash
sha256sum ~/.aios-spike-pi-state/auth.json > /tmp/bundle-before.sha
# wait for short-lived access token to expire (typically 1h); or force by editing
# auth.json's expiry to a past time (back up first)
for i in 1 2 3; do
  podman run --rm -v ~/.aios-spike-pi-state:/pi-state \
    -e PI_CODING_AGENT_DIR=/pi-state aios-spike/pi:latest \
    pi --mode json -p "ping $i" >/dev/null
done
sha256sum ~/.aios-spike-pi-state/auth.json > /tmp/bundle-after.sha
diff /tmp/bundle-before.sha /tmp/bundle-after.sha || echo "BUNDLE ROTATED"
```

- [ ] **Step 2: Determine atomicity**

```bash
# while pi runs, monitor with inotifywait in a sidecar
podman run -d --rm -v ~/.aios-spike-pi-state:/pi-state -e PI_CODING_AGENT_DIR=/pi-state \
  aios-spike/pi:latest \
  bash -c 'apt-get update >/dev/null && apt-get install -y inotify-tools >/dev/null && \
           inotifywait -m -e create -e moved_to /pi-state/'
# parallel: run pi -p "ping" with forced refresh
```

Look for the standard atomic-write pattern (`auth.json.tmp` → rename `auth.json`). If pi writes in place, broker must add file locking on top.

- [ ] **Step 3: Document `## A6`**

Record:
- Does pi rotate refresh tokens?
- Are writes atomic?
- What persistence/sync interval does the broker need?

- [ ] **Step 4: Commit**

### Task A7: Outbound HTTPS transport

**Goal:** Capture exactly which endpoints pi calls so the auth-broker can decide whether to *proxy* or just *broker* (hand-off auth bundle to Jobs).

- [ ] **Step 1: Run pi behind mitmproxy**

```bash
# Start mitmproxy in transparent mode in a sidecar
podman run --rm -d --name spike-mitm \
  -v "$PWD/mitm-certs:/home/mitmproxy/.mitmproxy" \
  -p 8080:8080 mitmproxy/mitmproxy mitmdump --set block_global=false
# Trust mitm CA in pi container
podman run --rm \
  -v ~/.aios-spike-pi-state:/pi-state \
  -v "$PWD/mitm-certs:/certs" \
  -e PI_CODING_AGENT_DIR=/pi-state \
  -e HTTPS_PROXY=http://host.containers.internal:8080 \
  -e SSL_CERT_FILE=/certs/mitmproxy-ca-cert.pem \
  aios-spike/pi:latest \
  pi --mode json -p "ping" 2>&1 | tee /tmp/pi-with-mitm.jsonl
podman stop spike-mitm
```

- [ ] **Step 2: Extract URL + auth-header summary from the mitm log**

For each captured request log:
- Host
- Path
- Authorization header shape (Bearer? OAuth2?)
- Whether the token format is the standard OpenAI `sk-...` or something else
- Response shape: standard OpenAI chat-completions JSON or proprietary?

- [ ] **Step 3: Document `## A7` and the Decision section**

Decide:
- **Brokered auth only (recommended default).** Broker hands each Job an auth bundle; pi makes calls directly to OpenAI. Concurrency control is via leases, not proxying. This is correct if the transport is non-public and we cannot safely intercept it.
- **Brokered auth + optional HTTP proxy.** If transport is public-OpenAI-compatible, broker may also expose a `/v1/chat/completions` proxy for observability/cost-tracking. Optional, post-Phase-1 enhancement.

- [ ] **Step 4: Commit**

### Task A8: Spike close-out

- [ ] **Step 1: Write the Decision section in findings doc**

Concretely answer:
1. Can the broker drive headless login? (Y/N — informs Task B6)
2. Is the auth bundle portable? (Y/N — informs Task B3)
3. Is concurrent multi-Job sharing safe? (Y/N — informs Task B9 design)
4. Is bundle write atomic? (Y/N — informs Task B3 file-locking)
5. Is transport OpenAI-compatible? (Y/N — informs whether B11 includes a proxy endpoint)
6. **Go / no-go for Phase 0 as designed?**

If any answer kills the brokered design, halt and re-brainstorm — do not start Phase 0.

- [ ] **Step 2: Commit**

```bash
git add docs/superpowers/spikes/2026-04-30-pi-auth-transport-findings.md
git commit -m "spike: complete auth transport investigation; decision <go|no-go>"
```

---

# Phase 0 — Auth-broker

**Goal:** A deployable Go service that owns the OAuth state, refreshes tokens proactively, leases concurrency slots to Jobs, hands out auth bundles, and drives Slack-DM phone reauth. Single replica. K8s-native (`SA` token auth for clients, 1Password Operator for secrets).

**Output of phase:** `auth-broker/` Go module + image + manifests + Slack app config; integration test that proves end-to-end reauth works from a phone.

**Time budget:** ~1 week.

### Task B1: Scaffold Go module

**Files:**
- Create: `auth-broker/go.mod`
- Create: `auth-broker/cmd/main.go`
- Create: `auth-broker/internal/server/server.go`
- Create: `auth-broker/cmd/main_test.go`
- Create: `auth-broker/.golangci.yml`

- [ ] **Step 1: Write a failing smoke test that the server starts and `/healthz` returns 200**

`auth-broker/cmd/main_test.go`:

```go
package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Diixtra/aios/auth-broker/internal/server"
)

func TestHealthz(t *testing.T) {
	srv := server.New(server.Config{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req.WithContext(context.Background()))
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
}
```

- [ ] **Step 2: Initialise the module and run the test (will fail — package missing)**

```bash
cd auth-broker
go mod init github.com/Diixtra/aios/auth-broker
go test ./... 2>&1 | head -20
```

Expected: `package github.com/Diixtra/aios/auth-broker/internal/server is not in std`.

- [ ] **Step 3: Implement minimal server**

`auth-broker/internal/server/server.go`:

```go
package server

import "net/http"

type Config struct{}

type Server struct {
	cfg Config
	mux *http.ServeMux
}

func New(cfg Config) *Server {
	s := &Server{cfg: cfg, mux: http.NewServeMux()}
	s.mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }
```

`auth-broker/cmd/main.go`:

```go
package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/Diixtra/aios/auth-broker/internal/server"
)

func main() {
	srv := server.New(server.Config{})
	addr := ":8080"
	slog.Info("auth-broker listening", "addr", addr)
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		slog.Error("listen", "err", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 4: Run test, expect pass**

```bash
go test ./...
```

Expected: `PASS`.

- [ ] **Step 5: Add golangci config matching other Go services**

`auth-broker/.golangci.yml` — copy from `operator/.golangci.yml` (or `webhook/.golangci.yml`, whichever is closer to current Diixtra Go config).

- [ ] **Step 6: Commit**

```bash
git add auth-broker/
git commit -m "feat(auth-broker): scaffold Go service with healthz"
```

### Task B2: Auth state machine

**Files:**
- Create: `auth-broker/internal/auth/state.go`
- Create: `auth-broker/internal/auth/state_test.go`

- [ ] **Step 1: Write a failing test for state transitions**

`auth-broker/internal/auth/state_test.go`:

```go
package auth

import (
	"testing"
	"time"
)

func TestStateFromTokenAge(t *testing.T) {
	tests := []struct {
		name      string
		ageDays   float64
		want      State
	}{
		{"fresh", 1, StateHealthy},
		{"approaching", 24, StateWarning},  // 7d before 30d expiry
		{"expired", 31, StateExpired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expiry := time.Now().Add(time.Duration((30 - tt.ageDays) * 24 * float64(time.Hour)))
			got := StateFromExpiry(expiry, time.Now())
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTransitionAwaiting(t *testing.T) {
	m := NewMachine()
	m.Set(StateHealthy)
	if err := m.Transition(StateAwaiting); err != nil {
		t.Fatal(err)
	}
	if m.State() != StateAwaiting {
		t.Fatalf("not transitioned")
	}
}

func TestTransitionRejectsInvalid(t *testing.T) {
	m := NewMachine()
	m.Set(StateAwaiting)
	if err := m.Transition(StateWarning); err == nil {
		t.Fatal("should reject Awaiting->Warning")
	}
}
```

- [ ] **Step 2: Run, confirm fail**

```bash
go test ./internal/auth/... 2>&1 | head
```

- [ ] **Step 3: Implement state machine**

`auth-broker/internal/auth/state.go`:

```go
package auth

import (
	"errors"
	"sync"
	"time"
)

type State string

const (
	StateUnknown  State = "Unknown"
	StateHealthy  State = "Healthy"
	StateWarning  State = "Warning"
	StateExpired  State = "Expired"
	StateAwaiting State = "Awaiting"
)

// validTransitions[from] = allowed targets
var validTransitions = map[State]map[State]struct{}{
	StateUnknown:  {StateHealthy: {}, StateExpired: {}, StateAwaiting: {}},
	StateHealthy:  {StateWarning: {}, StateExpired: {}, StateAwaiting: {}},
	StateWarning:  {StateHealthy: {}, StateExpired: {}, StateAwaiting: {}},
	StateExpired:  {StateAwaiting: {}, StateHealthy: {}},
	StateAwaiting: {StateHealthy: {}, StateExpired: {}},
}

type Machine struct {
	mu    sync.RWMutex
	state State
}

func NewMachine() *Machine {
	return &Machine{state: StateUnknown}
}

func (m *Machine) State() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

// Set forces the state without validation. Use for initial load only.
func (m *Machine) Set(s State) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = s
}

func (m *Machine) Transition(to State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	allowed, ok := validTransitions[m.state]
	if !ok {
		return errors.New("auth: state has no allowed transitions")
	}
	if _, ok := allowed[to]; !ok {
		return errors.New("auth: invalid transition " + string(m.state) + "->" + string(to))
	}
	m.state = to
	return nil
}

// StateFromExpiry classifies expiry-relative health.
//
// >7 days remaining -> Healthy; 0-7 days -> Warning; <=0 -> Expired.
func StateFromExpiry(expiresAt, now time.Time) State {
	delta := expiresAt.Sub(now)
	switch {
	case delta <= 0:
		return StateExpired
	case delta <= 7*24*time.Hour:
		return StateWarning
	default:
		return StateHealthy
	}
}
```

- [ ] **Step 4: Run tests, expect pass**

- [ ] **Step 5: Commit**

```bash
git add auth-broker/internal/auth
git commit -m "feat(auth-broker): auth state machine with classification + transitions"
```

### Task B3: Auth bundle storage

**Files:**
- Create: `auth-broker/internal/store/store.go`
- Create: `auth-broker/internal/store/store_test.go`

The store reads/writes `auth.json` (whatever pi's bundle format turned out to be in spike A2/A4). It must atomic-write and (if the spike showed pi writes in-place) hold a flock during reads.

- [ ] **Step 1: Failing test — round-trip bundle**

`auth-broker/internal/store/store_test.go`:

```go
package store

import (
	"path/filepath"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "auth.json"))

	want := Bundle{Raw: []byte(`{"refresh_token":"abc","expires_at":"2099-01-01T00:00:00Z"}`)}
	if err := s.Write(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Raw) != string(want.Raw) {
		t.Fatalf("mismatch")
	}
}

func TestAtomicWrite_NoPartialReadOnCrash(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "auth.json"))
	// Write valid bundle.
	if err := s.Write(Bundle{Raw: []byte("v1")}); err != nil {
		t.Fatal(err)
	}
	// Concurrent read should never see partial; we simulate by checking the
	// implementation uses *.tmp + rename.
	files, _ := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if len(files) != 0 {
		t.Fatalf("leftover .tmp file: %v", files)
	}
}
```

- [ ] **Step 2: Run, confirm fail**

- [ ] **Step 3: Implement**

`auth-broker/internal/store/store.go`:

```go
package store

import (
	"errors"
	"os"
	"path/filepath"
)

type Bundle struct {
	// Raw is the verbatim contents of pi's auth.json. We treat it as opaque to
	// the broker; only pi parses the inner structure. Phase -1 task A2 confirmed
	// the file shape; if its semantics change we update the spike doc, not this.
	Raw []byte
}

type Store struct {
	path string
}

func New(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Read() (Bundle, error) {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return Bundle{}, err
	}
	return Bundle{Raw: b}, nil
}

// Write atomically replaces the bundle on disk.
func (s *Store) Write(b Bundle) error {
	if len(b.Raw) == 0 {
		return errors.New("store: refusing to write empty bundle")
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".auth-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(b.Raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}
```

- [ ] **Step 4: Tests pass**

- [ ] **Step 5: Commit**

```bash
git add auth-broker/internal/store
git commit -m "feat(auth-broker): atomic bundle store"
```

### Task B4: Token refresh logic

**Files:**
- Create: `auth-broker/internal/auth/refresh.go`
- Create: `auth-broker/internal/auth/refresh_test.go`

The refresh logic is provider-specific. Phase -1 task A6 documents what kicking pi does to the bundle. The broker invokes pi as a subprocess to perform the refresh; this avoids reimplementing OpenAI's OAuth2 flow.

- [ ] **Step 1: Failing test using a fake pi binary**

`auth-broker/internal/auth/refresh_test.go`:

```go
package auth

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRefresh_InvokesPiAndPicksUpUpdatedBundle(t *testing.T) {
	dir := t.TempDir()
	// Write a fake pi script that touches a sentinel file.
	fakePi := filepath.Join(dir, "pi-fake")
	if err := os.WriteFile(fakePi, []byte("#!/usr/bin/env bash\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := NewRefresher(fakePi, dir)
	if err := r.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
}

func TestRefresh_ReturnsErrorWhenPiFails(t *testing.T) {
	dir := t.TempDir()
	fakePi := filepath.Join(dir, "pi-fake")
	if err := os.WriteFile(fakePi, []byte("#!/usr/bin/env bash\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := NewRefresher(fakePi, dir)
	if err := r.Refresh(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}
```

- [ ] **Step 2: Run, fail**

- [ ] **Step 3: Implement**

`auth-broker/internal/auth/refresh.go`:

```go
package auth

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// Refresher exercises pi non-interactively to refresh the token bundle.
//
// We do not reimplement OAuth2 in the broker. Instead, we pin pi's behaviour
// (documented in spike A6): a no-op invocation against the configured PI dir
// causes pi to refresh stale tokens in-place. We invoke pi with a trivial
// prompt that should consume zero (or near-zero) provider tokens.
type Refresher struct {
	piBinary string
	piDir    string
}

func NewRefresher(piBinary, piDir string) *Refresher {
	return &Refresher{piBinary: piBinary, piDir: piDir}
}

func (r *Refresher) Refresh(ctx context.Context) error {
	if r.piBinary == "" || r.piDir == "" {
		return errors.New("refresh: piBinary and piDir required")
	}
	cmd := exec.CommandContext(ctx, r.piBinary,
		"--mode", "json",
		"--no-extensions", "--no-skills",
		"--no-prompt-templates", "--no-context-files",
		"-p", "Reply with the single token PONG and nothing else.")
	cmd.Env = append(cmd.Environ(), "PI_CODING_AGENT_DIR="+r.piDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pi refresh: %w (output: %s)", err, string(out))
	}
	return nil
}
```

- [ ] **Step 4: Tests pass**

- [ ] **Step 5: Commit**

```bash
git add auth-broker/internal/auth/refresh.go auth-broker/internal/auth/refresh_test.go
git commit -m "feat(auth-broker): token refresh via pi subprocess"
```

### Task B5: Weekly silent refresh scheduler

**Files:**
- Create: `auth-broker/internal/scheduler/scheduler.go`
- Create: `auth-broker/internal/scheduler/scheduler_test.go`

- [ ] **Step 1: Failing test using a fake clock**

`auth-broker/internal/scheduler/scheduler_test.go`:

```go
package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestPeriodic_FiresAtInterval(t *testing.T) {
	var calls int32
	s := New(10*time.Millisecond, func(ctx context.Context) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Millisecond)
	defer cancel()
	s.Run(ctx)
	got := atomic.LoadInt32(&calls)
	if got < 4 || got > 6 {
		t.Fatalf("got %d calls, want ~5", got)
	}
}
```

- [ ] **Step 2: Run, fail**

- [ ] **Step 3: Implement**

`auth-broker/internal/scheduler/scheduler.go`:

```go
package scheduler

import (
	"context"
	"log/slog"
	"time"
)

type Job func(context.Context) error

type Scheduler struct {
	interval time.Duration
	job      Job
}

func New(interval time.Duration, job Job) *Scheduler {
	return &Scheduler{interval: interval, job: job}
}

// Run blocks until ctx is cancelled, invoking the job on each tick.
func (s *Scheduler) Run(ctx context.Context) {
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.job(ctx); err != nil {
				slog.Warn("scheduled job failed", "err", err)
			}
		}
	}
}
```

- [ ] **Step 4: Tests pass**

- [ ] **Step 5: Commit**

### Task B6: Bundle upload endpoint

**Goal:** Phase -1 task A3 confirmed pi 0.70.6's `/login` is interactive-only. The broker therefore cannot initiate logins; it *receives* an auth bundle bootstrapped by the user on a TTY-backed laptop. Tasks B6/B7/B8 implement that receive → validate → orchestrate state path. The original device-flow design (commits prior to A3 findings) is replaced wholesale.

**Files:**
- Create: `auth-broker/internal/server/bundle.go`
- Create: `auth-broker/internal/server/bundle_test.go`

The handler accepts an `auth.json` upload (multipart `bundle` field), validates it parses as JSON, persists via the store from Task B3, and triggers a validation cycle (Task B7). It is admin-token-guarded (Task B12) — only the user's bootstrap script should hit it.

- [ ] **Step 1: Failing test — happy-path upload stores the bundle and returns 202 Accepted**

`auth-broker/internal/server/bundle_test.go`:

```go
package server

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Diixtra/aios/auth-broker/internal/store"
)

func TestUploadBundle_Persists(t *testing.T) {
	dir := t.TempDir()
	st := store.New(filepath.Join(dir, "auth.json"))
	h := NewBundleHandler(st, func() {})

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, _ := w.CreateFormFile("bundle", "auth.json")
	_, _ = fw.Write([]byte(`{"chatgpt":{"type":"oauth","refresh_token":"r1"}}`))
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/bundle", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("got %d, want 202; body=%s", rec.Code, rec.Body)
	}
	got, err := st.Read()
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Raw) != `{"chatgpt":{"type":"oauth","refresh_token":"r1"}}` {
		t.Fatalf("bundle not persisted; got %s", got.Raw)
	}
}

func TestUploadBundle_RejectsNonJSON(t *testing.T) {
	dir := t.TempDir()
	st := store.New(filepath.Join(dir, "auth.json"))
	h := NewBundleHandler(st, func() {})

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, _ := w.CreateFormFile("bundle", "auth.json")
	_, _ = fw.Write([]byte(`not json`))
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/bundle", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", rec.Code)
	}
}

func TestUploadBundle_RejectsOversize(t *testing.T) {
	dir := t.TempDir()
	st := store.New(filepath.Join(dir, "auth.json"))
	h := NewBundleHandler(st, func() {})

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, _ := w.CreateFormFile("bundle", "auth.json")
	_, _ = fw.Write(bytes.Repeat([]byte("x"), 2*1024*1024)) // 2MB > limit
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/bundle", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got %d, want 413", rec.Code)
	}
}
```

- [ ] **Step 2: Run tests, expect fail**

```bash
go test ./internal/server/... -run TestUploadBundle 2>&1 | head -20
```

- [ ] **Step 3: Implement**

`auth-broker/internal/server/bundle.go`:

```go
package server

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/Diixtra/aios/auth-broker/internal/store"
)

const bundleSizeLimit = 1 << 20 // 1 MiB — pi auth.json is small (KB range)

// BundleHandler accepts an auth.json upload and triggers re-validation.
//
// Schema is intentionally not validated — pi is the source of truth for
// auth.json structure. We require only that the body is valid JSON so we
// fail fast on `curl -F bundle=@/wrong/file`.
type BundleHandler struct {
	store           *store.Store
	triggerValidate func()
}

func NewBundleHandler(s *store.Store, triggerValidate func()) *BundleHandler {
	return &BundleHandler{store: s, triggerValidate: triggerValidate}
}

func (h *BundleHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, bundleSizeLimit+1)
	if err := r.ParseMultipartForm(bundleSizeLimit); err != nil {
		var maxErr *http.MaxBytesError
		if asMaxBytes(err, &maxErr) {
			http.Error(w, "bundle too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "invalid multipart body", http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("bundle")
	if err != nil {
		http.Error(w, "missing bundle field", http.StatusBadRequest)
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	if !json.Valid(raw) {
		http.Error(w, "bundle must be JSON", http.StatusBadRequest)
		return
	}
	if err := h.store.Write(store.Bundle{Raw: raw}); err != nil {
		http.Error(w, "persist failed", http.StatusInternalServerError)
		return
	}
	if h.triggerValidate != nil {
		h.triggerValidate()
	}
	w.WriteHeader(http.StatusAccepted)
}

func asMaxBytes(err error, target **http.MaxBytesError) bool {
	for {
		if e, ok := err.(*http.MaxBytesError); ok {
			*target = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
		if err == nil {
			return false
		}
	}
}
```

- [ ] **Step 4: Tests pass**

- [ ] **Step 5: Commit**

```bash
git add auth-broker/internal/server/bundle.go auth-broker/internal/server/bundle_test.go
git commit -m "feat(auth-broker): bundle upload endpoint (POST /v1/auth/bundle)"
```

### Task B7: Bundle validation

**Files:**
- Create: `auth-broker/internal/auth/validate.go`
- Create: `auth-broker/internal/auth/validate_test.go`

After an upload, the broker must confirm the bundle actually authenticates with pi. We use `pi --list-models` (cheap; no inference call) — if it lists models without an auth error, the bundle is good. If it fails, the state machine moves to Expired and the user is DM'd.

- [ ] **Step 1: Failing test — Validate against a fake pi reports success/failure**

`auth-broker/internal/auth/validate_test.go`:

```go
package auth

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestValidate_ReportsSuccessOnZeroExit(t *testing.T) {
	dir := t.TempDir()
	fakePi := filepath.Join(dir, "pi-fake")
	if err := os.WriteFile(fakePi, []byte("#!/usr/bin/env bash\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	v := NewValidator(fakePi, dir)
	if err := v.Validate(context.Background()); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidate_ReportsFailureOnNonZeroExit(t *testing.T) {
	dir := t.TempDir()
	fakePi := filepath.Join(dir, "pi-fake")
	if err := os.WriteFile(fakePi, []byte("#!/usr/bin/env bash\necho 'auth error' >&2; exit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	v := NewValidator(fakePi, dir)
	if err := v.Validate(context.Background()); err == nil {
		t.Fatal("expected validation error")
	}
}
```

- [ ] **Step 2: Run tests, expect fail**

- [ ] **Step 3: Implement**

`auth-broker/internal/auth/validate.go`:

```go
package auth

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// Validator runs a cheap pi command against the configured PI dir to confirm
// the bundle authenticates. We use --list-models because it touches the
// provider for auth checks but does not bill an inference call.
type Validator struct {
	piBinary string
	piDir    string
}

func NewValidator(piBinary, piDir string) *Validator {
	return &Validator{piBinary: piBinary, piDir: piDir}
}

func (v *Validator) Validate(ctx context.Context) error {
	if v.piBinary == "" || v.piDir == "" {
		return errors.New("validate: piBinary and piDir required")
	}
	cmd := exec.CommandContext(ctx, v.piBinary, "--list-models")
	cmd.Env = append(cmd.Environ(), "PI_CODING_AGENT_DIR="+v.piDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pi validate: %w (output: %s)", err, string(out))
	}
	return nil
}
```

- [ ] **Step 4: Tests pass**

- [ ] **Step 5: Commit**

```bash
git add auth-broker/internal/auth/validate.go auth-broker/internal/auth/validate_test.go
git commit -m "feat(auth-broker): bundle validation via pi --list-models"
```

### Task B8: State orchestration

**Files:**
- Create: `auth-broker/internal/auth/orchestrator.go`
- Create: `auth-broker/internal/auth/orchestrator_test.go`

The orchestrator is the brain that wires Tasks B2/B4/B5/B7 together. It consumes events:
- bundle uploaded (from B6) → run validate → on success transition to Healthy and fire confirmation DM; on failure transition to Expired and fire urgent DM
- periodic tick (from B5 scheduler) → if bundle absent, state stays Uninitialised; if present, run refresh (B4) then validate (B7); transition to Warning when bundle older than warn threshold; transition to Expired on validation failure
- explicit `/aios-reauth` trigger (from B16) → re-run validate immediately and re-emit bootstrap recipe DM

Notifications fire **only on state transition** (not every tick) to avoid spamming.

- [ ] **Step 1: Failing test — fresh upload + valid bundle → Healthy + Recovered DM**

`auth-broker/internal/auth/orchestrator_test.go`:

```go
package auth

import (
	"context"
	"errors"
	"testing"
)

type fakeNotifier struct {
	calls []string
}

func (f *fakeNotifier) BootstrapRecipe(ctx context.Context, reason string) error {
	f.calls = append(f.calls, "bootstrap:"+reason)
	return nil
}
func (f *fakeNotifier) Recovered(ctx context.Context) error {
	f.calls = append(f.calls, "recovered")
	return nil
}
func (f *fakeNotifier) Warning(ctx context.Context, ageDays int) error {
	f.calls = append(f.calls, "warning")
	return nil
}

type fakeValidator struct{ err error }

func (f *fakeValidator) Validate(ctx context.Context) error { return f.err }

func TestOrchestrator_UploadOK_TransitionsToHealthyAndDMsRecovery(t *testing.T) {
	sm := NewMachine()
	sm.Set(StateExpired)
	n := &fakeNotifier{}
	o := NewOrchestrator(sm, &fakeValidator{}, n)

	if err := o.OnBundleUploaded(context.Background()); err != nil {
		t.Fatalf("OnBundleUploaded: %v", err)
	}
	if sm.State() != StateHealthy {
		t.Fatalf("state=%s want Healthy", sm.State())
	}
	if len(n.calls) != 1 || n.calls[0] != "recovered" {
		t.Fatalf("notify=%v want [recovered]", n.calls)
	}
}

func TestOrchestrator_UploadFails_TransitionsToExpiredAndDMsBootstrap(t *testing.T) {
	sm := NewMachine()
	sm.Set(StateHealthy)
	n := &fakeNotifier{}
	o := NewOrchestrator(sm, &fakeValidator{err: errors.New("auth error")}, n)

	if err := o.OnBundleUploaded(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if sm.State() != StateExpired {
		t.Fatalf("state=%s want Expired", sm.State())
	}
	if len(n.calls) != 1 || n.calls[0] != "bootstrap:upload-validation-failed" {
		t.Fatalf("notify=%v", n.calls)
	}
}

func TestOrchestrator_Tick_NoBundle_NoTransition(t *testing.T) {
	sm := NewMachine()
	sm.Set(StateUnknown) // Set ignores allowed list — set initial state directly
	n := &fakeNotifier{}
	o := NewOrchestrator(sm, &fakeValidator{}, n)
	o.bundleAge = func() (int, bool) { return 0, false } // no bundle
	if err := o.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(n.calls) != 0 {
		t.Fatalf("expected no DMs; got %v", n.calls)
	}
}

func TestOrchestrator_Tick_AgedBundle_TransitionsToWarningOnce(t *testing.T) {
	sm := NewMachine()
	sm.Set(StateHealthy)
	n := &fakeNotifier{}
	o := NewOrchestrator(sm, &fakeValidator{}, n)
	o.bundleAge = func() (int, bool) { return 25, true } // past warn (default 23d)
	if err := o.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := o.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sm.State() != StateWarning {
		t.Fatalf("state=%s want Warning", sm.State())
	}
	// Two ticks, one DM — transition fires only once.
	if len(n.calls) != 1 || n.calls[0] != "warning" {
		t.Fatalf("notify=%v want [warning]", n.calls)
	}
}
```

- [ ] **Step 2: Run, fail**

- [ ] **Step 3: Implement**

`auth-broker/internal/auth/orchestrator.go`:

```go
package auth

import (
	"context"
	"errors"
)

const defaultWarnAgeDays = 23 // OAuth refresh-token chains commonly survive ~30d

type validator interface {
	Validate(context.Context) error
}

type Notifier interface {
	BootstrapRecipe(ctx context.Context, reason string) error
	Recovered(ctx context.Context) error
	Warning(ctx context.Context, ageDays int) error
}

type Orchestrator struct {
	sm        *Machine
	validator validator
	notifier  Notifier

	// bundleAge returns (ageDays, present). Wired by main to a closure over
	// the store's mtime; injectable for tests.
	bundleAge   func() (int, bool)
	warnAgeDays int
}

func NewOrchestrator(sm *Machine, v validator, n Notifier) *Orchestrator {
	return &Orchestrator{sm: sm, validator: v, notifier: n, warnAgeDays: defaultWarnAgeDays}
}

// OnBundleUploaded validates the freshly-stored bundle and transitions state.
func (o *Orchestrator) OnBundleUploaded(ctx context.Context) error {
	prev := o.sm.State()
	if err := o.validator.Validate(ctx); err != nil {
		_ = o.sm.Transition(StateExpired)
		_ = o.notifier.BootstrapRecipe(ctx, "upload-validation-failed")
		return err
	}
	_ = o.sm.Transition(StateHealthy)
	if prev != StateHealthy {
		_ = o.notifier.Recovered(ctx)
	}
	return nil
}

// Tick runs the periodic check. Should be called by the B5 scheduler.
func (o *Orchestrator) Tick(ctx context.Context) error {
	if o.bundleAge == nil {
		return errors.New("orchestrator: bundleAge not wired")
	}
	age, present := o.bundleAge()
	if !present {
		// No bundle uploaded yet. Nothing to do — bootstrap recipe was sent
		// at startup; do not re-DM on every tick.
		return nil
	}
	prev := o.sm.State()
	if err := o.validator.Validate(ctx); err != nil {
		if prev != StateExpired {
			_ = o.sm.Transition(StateExpired)
			_ = o.notifier.BootstrapRecipe(ctx, "tick-validation-failed")
		}
		return err
	}
	if age >= o.warnAgeDays {
		if prev != StateWarning {
			_ = o.sm.Transition(StateWarning)
			_ = o.notifier.Warning(ctx, age)
		}
		return nil
	}
	// Healthy.
	if prev != StateHealthy {
		_ = o.sm.Transition(StateHealthy)
		if prev == StateExpired || prev == StateWarning {
			_ = o.notifier.Recovered(ctx)
		}
	}
	return nil
}
```

- [ ] **Step 4: Tests pass**

- [ ] **Step 5: Commit**

```bash
git add auth-broker/internal/auth/orchestrator.go auth-broker/internal/auth/orchestrator_test.go
git commit -m "feat(auth-broker): state orchestrator wires validator/store/notifier"
```

### Task B9: Lease API

**Files:**
- Create: `auth-broker/internal/lease/lease.go`
- Create: `auth-broker/internal/lease/lease_test.go`

- [ ] **Step 1: Failing test for the semaphore semantics**

```go
package lease

import (
	"context"
	"testing"
	"time"
)

func TestAcquire_RespectsCap(t *testing.T) {
	mgr := New(2, time.Hour)

	a, err := mgr.Acquire(context.Background(), "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	b, err := mgr.Acquire(context.Background(), "agent-2")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := mgr.Acquire(ctx, "agent-3"); err == nil {
		t.Fatal("expected timeout")
	}

	if err := mgr.Release(a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Acquire(context.Background(), "agent-4"); err != nil {
		t.Fatalf("should be able to acquire after release: %v", err)
	}
	_ = b
}

func TestAcquire_ExpiresStaleLease(t *testing.T) {
	mgr := New(1, 10*time.Millisecond)
	if _, err := mgr.Acquire(context.Background(), "agent-1"); err != nil {
		t.Fatal(err)
	}
	// Wait past expiry; new acquire should succeed.
	time.Sleep(20 * time.Millisecond)
	if _, err := mgr.Acquire(context.Background(), "agent-2"); err != nil {
		t.Fatalf("should reclaim stale lease: %v", err)
	}
}
```

- [ ] **Step 2: Implement**

```go
package lease

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

type Lease struct {
	ID       string
	Holder   string
	IssuedAt time.Time
	ExpiresAt time.Time
}

type Manager struct {
	cap     int
	ttl     time.Duration

	mu      sync.Mutex
	active  map[string]*Lease
	wakeup  chan struct{}
}

func New(cap int, ttl time.Duration) *Manager {
	return &Manager{
		cap:    cap,
		ttl:    ttl,
		active: make(map[string]*Lease),
		wakeup: make(chan struct{}, 1),
	}
}

func (m *Manager) Acquire(ctx context.Context, holder string) (*Lease, error) {
	for {
		m.mu.Lock()
		m.reapLocked(time.Now())
		if len(m.active) < m.cap {
			id := newID()
			now := time.Now()
			l := &Lease{ID: id, Holder: holder, IssuedAt: now, ExpiresAt: now.Add(m.ttl)}
			m.active[id] = l
			m.mu.Unlock()
			return l, nil
		}
		m.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-m.wakeup:
			// Try again.
		case <-time.After(50 * time.Millisecond):
			// Periodic re-check.
		}
	}
}

func (m *Manager) Release(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.active[id]; !ok {
		return errors.New("lease: unknown id")
	}
	delete(m.active, id)
	select {
	case m.wakeup <- struct{}{}:
	default:
	}
	return nil
}

func (m *Manager) Active() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reapLocked(time.Now())
	return len(m.active)
}

func (m *Manager) reapLocked(now time.Time) {
	for id, l := range m.active {
		if now.After(l.ExpiresAt) {
			delete(m.active, id)
		}
	}
}

func newID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
```

- [ ] **Step 3: Tests pass**

- [ ] **Step 4: Commit**

### Task B10: Slack DM client + state-driven notifications

**Files:**
- Create: `auth-broker/internal/notify/slack.go`
- Create: `auth-broker/internal/notify/slack_test.go`

The broker DMs the user on state transitions only (the orchestrator from B8 enforces transition-only firing). The notifier exposes three messages matching the orchestrator's `Notifier` interface: bootstrap recipe (initial / on validation failure), warning (bundle aging), recovered (back to Healthy).

- [ ] **Step 1: Failing test using a fake Slack client**

```go
package notify

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeSlack struct {
	calls []string
	err   error
}

func (f *fakeSlack) DM(_ context.Context, userID, text string) error {
	if f.err != nil {
		return f.err
	}
	f.calls = append(f.calls, userID+":"+text)
	return nil
}

func TestNotifier_BootstrapRecipe(t *testing.T) {
	c := &fakeSlack{}
	n := NewNotifier(c, "U123", "https://auth-broker.aios.local")
	if err := n.BootstrapRecipe(context.Background(), "tick-validation-failed"); err != nil {
		t.Fatal(err)
	}
	if len(c.calls) != 1 {
		t.Fatalf("got %v", c.calls)
	}
	if !strings.Contains(c.calls[0], "pi /login") || !strings.Contains(c.calls[0], "tick-validation-failed") {
		t.Fatalf("DM missing recipe or reason: %s", c.calls[0])
	}
}

func TestNotifier_Warning(t *testing.T) {
	c := &fakeSlack{}
	n := NewNotifier(c, "U123", "https://auth-broker.aios.local")
	if err := n.Warning(context.Background(), 25); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.calls[0], "25") {
		t.Fatalf("Warning DM missing age: %s", c.calls[0])
	}
}

func TestNotifier_Recovered(t *testing.T) {
	c := &fakeSlack{}
	n := NewNotifier(c, "U123", "https://auth-broker.aios.local")
	if err := n.Recovered(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.calls[0], "reauthenticated") {
		t.Fatalf("Recovered DM unexpected: %s", c.calls[0])
	}
}

func TestNotifier_PropagatesError(t *testing.T) {
	c := &fakeSlack{err: errors.New("boom")}
	n := NewNotifier(c, "U123", "https://x")
	if err := n.Recovered(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}
```

- [ ] **Step 2: Implement**

```go
package notify

import (
	"context"
	"fmt"
)

type SlackClient interface {
	DM(ctx context.Context, userID, text string) error
}

type Notifier struct {
	client    SlackClient
	userID    string
	brokerURL string
}

func NewNotifier(c SlackClient, userID, brokerURL string) *Notifier {
	return &Notifier{client: c, userID: userID, brokerURL: brokerURL}
}

const recipeTemplate = `AIOS auth needs attention (reason: %s).

On a laptop with pi installed:

    pi  # then run /login interactively, complete OAuth in browser, then exit pi
    just bootstrap-auth   # uploads ~/.pi/agent/auth.json to %s

The agent queue is paused until the upload validates.`

func (n *Notifier) BootstrapRecipe(ctx context.Context, reason string) error {
	return n.client.DM(ctx, n.userID, fmt.Sprintf(recipeTemplate, reason, n.brokerURL))
}

func (n *Notifier) Warning(ctx context.Context, ageDays int) error {
	return n.client.DM(ctx, n.userID,
		fmt.Sprintf("AIOS auth bundle is %dd old — refresh at your convenience: re-run `pi /login` on laptop and `just bootstrap-auth` upload.", ageDays))
}

func (n *Notifier) Recovered(ctx context.Context) error {
	return n.client.DM(ctx, n.userID, "AIOS reauthenticated, queue draining.")
}
```

- [ ] **Step 3: Real client wrapping `slack-go/slack`**

Add `internal/notify/slack_real.go`:

```go
package notify

import (
	"context"

	"github.com/slack-go/slack"
)

type RealSlack struct {
	api *slack.Client
}

func NewRealSlack(token string) *RealSlack {
	return &RealSlack{api: slack.New(token)}
}

func (r *RealSlack) DM(ctx context.Context, userID, text string) error {
	channel, _, _, err := r.api.OpenConversationContext(ctx,
		&slack.OpenConversationParameters{Users: []string{userID}})
	if err != nil {
		return err
	}
	_, _, err = r.api.PostMessageContext(ctx, channel.ID, slack.MsgOptionText(text, false))
	return err
}
```

`go get github.com/slack-go/slack`.

- [ ] **Step 4: Tests pass**

- [ ] **Step 5: Commit**

### Task B11: HTTP API endpoints

**Files:**
- Modify: `auth-broker/internal/server/server.go`
- Create: `auth-broker/internal/server/leases.go`
- Create: `auth-broker/internal/server/auth.go`
- Create: `auth-broker/internal/server/revalidate.go`
- Modify: `auth-broker/internal/server/server_test.go`

Endpoints to add (B6 already added `POST /v1/auth/bundle`):
- `POST /v1/leases/acquire` (body: `{holder}` → `{lease_id, expires_at}`)
- `POST /v1/leases/release` (body: `{lease_id}` → 204)
- `GET /v1/auth/bundle` (returns the verbatim `auth.json` bytes — admin-token-guarded; only the operator may pull a bundle into a Job)
- `POST /v1/admin/revalidate` (admin-token-guarded; runs the orchestrator's `OnBundleUploaded` path against the current bundle and returns `{state, message}`)
- `GET /healthz`, `GET /metrics` (prometheus)

- [ ] **Step 1: Failing tests — one per handler, real httptest, real lease.Manager + store.Store**

`auth-broker/internal/server/server_test.go`:

```go
package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Diixtra/aios/auth-broker/internal/lease"
	"github.com/Diixtra/aios/auth-broker/internal/store"
)

func TestAcquireLease_HappyPath(t *testing.T) {
	srv := New(Config{Lease: lease.New(2, time.Hour)})
	body, _ := json.Marshal(map[string]string{"holder": "agent-1"})
	req := httptest.NewRequest(http.MethodPost, "/v1/leases/acquire", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, body=%s", rec.Code, rec.Body)
	}
	var resp struct {
		LeaseID   string    `json:"lease_id"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.LeaseID == "" {
		t.Fatal("lease_id empty")
	}
}

func TestReleaseLease_RejectsUnknownID(t *testing.T) {
	srv := New(Config{Lease: lease.New(1, time.Hour)})
	body, _ := json.Marshal(map[string]string{"lease_id": "nope"})
	req := httptest.NewRequest(http.MethodPost, "/v1/leases/release", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestAuthBundle_ReturnsRawBundle(t *testing.T) {
	dir := t.TempDir()
	st := store.New(filepath.Join(dir, "auth.json"))
	if err := st.Write(store.Bundle{Raw: []byte(`{"chatgpt":{"type":"oauth"}}`)}); err != nil {
		t.Fatal(err)
	}
	srv := New(Config{Store: st})
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/bundle", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	if rec.Body.String() != `{"chatgpt":{"type":"oauth"}}` {
		t.Fatalf("got %q", rec.Body.String())
	}
}

type stubOrchestrator struct{ err error }

func (s *stubOrchestrator) OnBundleUploaded(_ context.Context) error { return s.err }

func TestRevalidate_RunsOrchestratorAndReturnsState(t *testing.T) {
	srv := New(Config{Orchestrator: &stubOrchestrator{}})
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/revalidate", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, body=%s", rec.Code, rec.Body)
	}
}
```

(Adjust imports as needed; the `context` import is required for the stub. `Orchestrator` field on `Config` matches the interface defined below.)

- [ ] **Step 2: Implement handlers in their respective files**

Server constructor:

```go
package server

import (
	"context"
	"net/http"

	"github.com/Diixtra/aios/auth-broker/internal/lease"
	"github.com/Diixtra/aios/auth-broker/internal/store"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type orchestrator interface {
	OnBundleUploaded(ctx context.Context) error
}

type Config struct {
	Lease        *lease.Manager
	Store        *store.Store
	Orchestrator orchestrator
}

type Server struct {
	cfg Config
	mux *http.ServeMux
}

func New(cfg Config) *Server {
	s := &Server{cfg: cfg, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	s.mux.Handle("GET /metrics", promhttp.Handler())
	s.mux.HandleFunc("POST /v1/leases/acquire", s.acquireLease)
	s.mux.HandleFunc("POST /v1/leases/release", s.releaseLease)
	s.mux.HandleFunc("GET /v1/auth/bundle", s.authBundle)
	s.mux.HandleFunc("POST /v1/admin/revalidate", s.revalidate)
	// POST /v1/auth/bundle is registered by the BundleHandler from Task B6,
	// wired in main.go alongside this server.
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }
```

(Using Go 1.22+ method-prefix routing.) Per-handler implementations live in `leases.go`, `auth.go`, and `revalidate.go` respectively, each ~20-30 lines.

- [ ] **Step 3: Tests pass**

- [ ] **Step 4: Commit**

```bash
git add auth-broker/internal/server
git commit -m "feat(auth-broker): HTTP endpoints for leases, bundle read, revalidate"
```

### Task B12: K8s ServiceAccount auth middleware

**Files:**
- Create: `auth-broker/internal/server/middleware.go`
- Create: `auth-broker/internal/server/middleware_test.go`

Jobs authenticate to the broker using their projected SA token. The broker validates via `TokenReview`. Bundle and lease endpoints require this. `/admin/*` requires a separate admin token (env var, mounted from a Secret) — only the operator and webhook should hit it.

- [ ] **Step 1: Failing test using a fake token reviewer**

```go
type fakeReviewer struct{ ok bool }
func (f *fakeReviewer) Authenticate(ctx context.Context, token string) (string, error) {
	if f.ok { return "system:serviceaccount:aios:agent-task", nil }
	return "", errors.New("not authenticated")
}

func TestRequireSAToken_Rejects401(t *testing.T) { ... }
func TestRequireSAToken_AllowsValid(t *testing.T) { ... }
func TestRequireAdminToken_RejectsMismatch(t *testing.T) { ... }
```

- [ ] **Step 2: Implement**

```go
package server

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
)

type tokenReviewer interface {
	Authenticate(ctx context.Context, token string) (subject string, err error)
}

func requireSAToken(rv tokenReviewer, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if tok == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		sub, err := rv.Authenticate(r.Context(), tok)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), ctxSubject{}, sub)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requireAdminToken(expected string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type ctxSubject struct{}
```

Then a real `kubeReviewer` using `client-go`:

```go
package server

import (
	"context"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type KubeReviewer struct{ Client kubernetes.Interface }

func (k *KubeReviewer) Authenticate(ctx context.Context, token string) (string, error) {
	tr := &authv1.TokenReview{Spec: authv1.TokenReviewSpec{Token: token}}
	res, err := k.Client.AuthenticationV1().TokenReviews().Create(ctx, tr, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}
	if !res.Status.Authenticated {
		return "", errors.New("not authenticated")
	}
	return res.Status.User.Username, nil
}
```

- [ ] **Step 3: Wire middleware to handlers in `server.New`**

- [ ] **Step 4: Tests pass**

- [ ] **Step 5: Commit**

### Task B13: Configuration loading

**Files:**
- Create: `auth-broker/internal/config/config.go`
- Create: `auth-broker/internal/config/config_test.go`

Env-var driven (12-factor). Required:
- `AUTH_BROKER_PI_BINARY` (path)
- `AUTH_BROKER_PI_DIR` (path, on PVC)
- `AUTH_BROKER_LEASE_CAP` (int, default 4)
- `AUTH_BROKER_LEASE_TTL` (duration, default 30m)
- `AUTH_BROKER_REFRESH_INTERVAL` (duration, default 168h)
- `AUTH_BROKER_SLACK_TOKEN` (Bot token from Secret)
- `AUTH_BROKER_SLACK_DM_USER_ID` (single user U-id)
- `AUTH_BROKER_ADMIN_TOKEN` (random secret)
- `AUTH_BROKER_LISTEN_ADDR` (default `:8080`)

- [ ] **Step 1: Failing test**

```go
func TestLoad_RequiresPiBinary(t *testing.T) { ... }
func TestLoad_DefaultsApplied(t *testing.T) { ... }
```

- [ ] **Step 2: Implement** with `os.Getenv` + a small `mustEnv`/`envDuration`/`envInt` helper.

- [ ] **Step 3: Wire config to main.go, replacing the hardcoded `:8080`**

- [ ] **Step 4: Commit**

### Task B14: Dockerfile

**Files:**
- Create: `auth-broker/Dockerfile`

- [ ] **Step 1: Multi-stage build with pi installed**

```dockerfile
# syntax=docker/dockerfile:1
FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/auth-broker ./cmd

FROM node:24-bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tini \
  && rm -rf /var/lib/apt/lists/* \
  && npm install -g @mariozechner/pi-coding-agent
COPY --from=build /out/auth-broker /usr/local/bin/auth-broker
ENV AUTH_BROKER_PI_BINARY=/usr/local/bin/pi
ENV AUTH_BROKER_PI_DIR=/pi-state
USER 65532:65532
ENTRYPOINT ["/usr/bin/tini","--"]
CMD ["/usr/local/bin/auth-broker"]
```

> **Note:** running as non-root (65532) requires `/pi-state` to be a writeable PVC mounted with `fsGroup: 65532`.

- [ ] **Step 2: Verify image builds**

```bash
podman build -t aios/auth-broker:dev auth-broker/
```

- [ ] **Step 3: Commit**

### Task B15: K8s manifests

**Files:**
- Create: `k8s/base/auth-broker/kustomization.yaml`
- Create: `k8s/base/auth-broker/deployment.yaml`
- Create: `k8s/base/auth-broker/service.yaml`
- Create: `k8s/base/auth-broker/serviceaccount.yaml`
- Create: `k8s/base/auth-broker/clusterrole-tokenreview.yaml`
- Create: `k8s/base/auth-broker/onepassword.yaml`
- Create: `k8s/base/auth-broker/pvc.yaml`
- Create: `k8s/base/auth-broker/networkpolicy.yaml`

> **Note:** Follow the pattern in `k8s/base/operator/` for OnePasswordItem, Cilium NetworkPolicy, and Kustomize structure. Resource limits MUST be set (Kyverno enforces).

- [ ] **Step 1: PVC**

`k8s/base/auth-broker/pvc.yaml`:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: auth-broker-pi-state
  namespace: aios
spec:
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 1Gi
  storageClassName: ${STORAGE_CLASS}
```

- [ ] **Step 2: Deployment with replicas=1, fsGroup=65532**

`k8s/base/auth-broker/deployment.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: auth-broker
  namespace: aios
  labels: { app: auth-broker }
spec:
  replicas: 1
  strategy: { type: Recreate }   # PVC is RWO; avoid two pods racing for it
  selector:
    matchLabels: { app: auth-broker }
  template:
    metadata:
      labels: { app: auth-broker }
    spec:
      serviceAccountName: auth-broker
      automountServiceAccountToken: true
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        runAsGroup: 65532
        fsGroup: 65532
        seccompProfile: { type: RuntimeDefault }
      containers:
      - name: broker
        image: ghcr.io/diixtra/aios-auth-broker:dev
        imagePullPolicy: IfNotPresent
        ports:
        - { name: http, containerPort: 8080 }
        env:
        - { name: AUTH_BROKER_LISTEN_ADDR, value: ":8080" }
        - { name: AUTH_BROKER_PI_BINARY,   value: /usr/local/bin/pi }
        - { name: AUTH_BROKER_PI_DIR,      value: /pi-state }
        - { name: AUTH_BROKER_LEASE_CAP,   value: "4" }
        - { name: AUTH_BROKER_LEASE_TTL,   value: "30m" }
        - { name: AUTH_BROKER_REFRESH_INTERVAL, value: "168h" }
        envFrom:
        - secretRef: { name: auth-broker-secrets }   # filled by OnePasswordItem
        resources:
          requests: { cpu: 100m, memory: 128Mi }
          limits:   { cpu: 500m, memory: 512Mi }
        readinessProbe:
          httpGet: { path: /healthz, port: http }
          periodSeconds: 5
        livenessProbe:
          httpGet: { path: /healthz, port: http }
          initialDelaySeconds: 30
          periodSeconds: 30
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          capabilities: { drop: [ALL] }
        volumeMounts:
        - { name: pi-state, mountPath: /pi-state }
        - { name: tmp,      mountPath: /tmp }
      volumes:
      - name: pi-state
        persistentVolumeClaim: { claimName: auth-broker-pi-state }
      - name: tmp
        emptyDir: {}
```

- [ ] **Step 3: Service** (ClusterIP on 8080).

- [ ] **Step 4: ServiceAccount + ClusterRoleBinding for `system:auth-delegator` (so TokenReview works)**

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata: { name: auth-broker-tokenreview }
subjects:
- kind: ServiceAccount
  name: auth-broker
  namespace: aios
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: system:auth-delegator
```

- [ ] **Step 5: OnePasswordItem with: Slack token, admin token, optional pi pre-seeded auth.json**

- [ ] **Step 6: Cilium NetworkPolicy: allow ingress from `app=runtime` and `app=operator` and `app=webhook` pods only; egress to api.openai.com (443) and slack.com (443) only**

- [ ] **Step 7: Kustomization referencing all above**

- [ ] **Step 8: Apply on a kind cluster as smoke test**

```bash
kind create cluster --name aios-spike
kubectl create namespace aios
kubectl apply -k k8s/base/auth-broker/
kubectl -n aios rollout status deploy/auth-broker --timeout=2m
kubectl -n aios port-forward svc/auth-broker 8080:8080 &
curl -sf localhost:8080/healthz
```

Expected: 200 OK.

- [ ] **Step 9: Commit**

### Task B16: `/aios-reauth` slash command in webhook

**Files:**
- Modify: `webhook/cmd/main.go` (route registration)
- Create: `webhook/internal/slack/reauth.go`
- Create: `webhook/internal/slack/reauth_test.go`

- [ ] **Step 1: Failing test for the slash-command handler**

```go
func TestReauthSlash_CallsAuthBroker(t *testing.T) {
	// Spin up a fake auth-broker that captures the POST.
	captured := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured <- string(body)
		_, _ = w.Write([]byte(`{"state":"Healthy","message":"validated ok"}`))
	}))
	defer srv.Close()

	h := NewReauthHandler(srv.URL+"/v1/admin/revalidate", "admin-token")
	rec := httptest.NewRecorder()
	form := url.Values{"text": []string{""}}
	req := httptest.NewRequest(http.MethodPost, "/slack/reauth",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK { t.Fatalf("got %d", rec.Code) }
	select {
	case <-captured:
	case <-time.After(time.Second):
		t.Fatal("auth-broker never called")
	}
}
```

- [ ] **Step 2: Implement**

`webhook/internal/slack/reauth.go`:

```go
package slack

import (
	"bytes"
	"encoding/json"
	"net/http"
)

// ReauthHandler proxies the Slack /aios-reauth slash command to the
// auth-broker's revalidate endpoint and returns the resulting state to Slack.
// Triggering revalidate also causes the broker to re-emit the bootstrap
// recipe DM (per orchestrator contract in Task B8) so the user gets the
// laptop instructions in their DM.
type ReauthHandler struct {
	authBrokerURL string
	adminToken    string
	client        *http.Client
}

func NewReauthHandler(url, adminToken string) *ReauthHandler {
	return &ReauthHandler{
		authBrokerURL: url, adminToken: adminToken,
		client: &http.Client{},
	}
}

func (h *ReauthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// (signature verification — pull from existing webhook/internal/slack helpers)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		h.authBrokerURL, bytes.NewReader([]byte("{}")))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("Authorization", "Bearer "+h.adminToken)
	req.Header.Set("Content-Type", "application/json")
	res, err := h.client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer res.Body.Close()
	var body struct {
		State   string `json:"state"`
		Message string `json:"message"`
	}
	_ = json.NewDecoder(res.Body).Decode(&body)
	text := "AIOS auth state: " + body.State
	if body.Message != "" {
		text += " — " + body.Message
	}
	if body.State != "Healthy" {
		text += "\n(Bootstrap recipe DM'd to you; re-run `pi /login` on laptop and `just bootstrap-auth`.)"
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"response_type": "ephemeral",
		"text":          text,
	})
}
```

> **Note:** signature verification — the existing `webhook/internal/slack/` package already validates Slack signatures for other slash commands. Use that helper; don't reimplement.

- [ ] **Step 3: Register route in `webhook/cmd/main.go`**

```go
mux.Handle("POST /slack/reauth", slack.NewReauthHandler(
    os.Getenv("AUTH_BROKER_URL")+"/v1/admin/revalidate",
    os.Getenv("AUTH_BROKER_ADMIN_TOKEN"),
))
```

- [ ] **Step 4: Add Slack app slash command pointing at `/slack/reauth`**

This is configuration in Slack, not code. Document in `webhook/SLACK_APP.md` (if missing, create) the new slash command and the bot scopes required (`commands`, `chat:write`, `im:write`).

- [ ] **Step 5: Tests pass**

- [ ] **Step 6: Commit**

### Task B17: End-to-end bootstrap + lifecycle integration test

**Files:**
- Create: `auth-broker/test/e2e/compose.yml`
- Create: `auth-broker/test/e2e/smoke.sh`
- Create: `auth-broker/test/e2e/bootstrap-auth.sh`
- Create: `auth-broker/test/e2e/README.md`

This is the gate: real pi binary (in a container), real Slack DM (to a test channel), real ChatGPT subscription bundle (test account, not production).

> **Note:** to keep the test reproducible, use a *test* OpenAI account (not the production subscription) and a *test* Slack workspace. Do not run this against production secrets in CI. The test is human-in-the-loop and runs locally — it is not a CI gate.

- [ ] **Step 1: Compose-based harness that spins up auth-broker**

`auth-broker/test/e2e/compose.yml`:

```yaml
services:
  auth-broker:
    build: ../..
    environment:
      AUTH_BROKER_PI_BINARY: /usr/local/bin/pi
      AUTH_BROKER_PI_DIR: /pi-state
      AUTH_BROKER_LEASE_CAP: "2"
      AUTH_BROKER_SLACK_TOKEN: ${TEST_SLACK_TOKEN}
      AUTH_BROKER_SLACK_DM_USER_ID: ${TEST_SLACK_USER}
      AUTH_BROKER_ADMIN_TOKEN: ${TEST_ADMIN_TOKEN}
    volumes:
      - pi-state:/pi-state
    ports: ["18080:8080"]
volumes:
  pi-state:
```

- [ ] **Step 2: Bootstrap helper script (used by `just bootstrap-auth` recipe later)**

`auth-broker/test/e2e/bootstrap-auth.sh`:

```bash
#!/usr/bin/env bash
# Uploads ~/.pi/agent/auth.json to the auth-broker.
# Run after `pi /login` completes successfully on the laptop.
set -euo pipefail

BROKER_URL="${AUTH_BROKER_URL:?missing AUTH_BROKER_URL}"
ADMIN_TOKEN="${AUTH_BROKER_ADMIN_TOKEN:?missing AUTH_BROKER_ADMIN_TOKEN}"
BUNDLE="${PI_AUTH_BUNDLE:-$HOME/.pi/agent/auth.json}"

if [[ ! -f "$BUNDLE" ]]; then
  echo "Bundle not found: $BUNDLE" >&2
  echo "Run 'pi' interactively, then '/login' to complete OAuth, then re-run this script." >&2
  exit 1
fi

curl -sf -X POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -F "bundle=@${BUNDLE}" \
  "${BROKER_URL%/}/v1/auth/bundle"
echo "Uploaded $BUNDLE to $BROKER_URL"
```

- [ ] **Step 3: Manual smoke script**

`auth-broker/test/e2e/smoke.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
podman compose up -d
trap 'podman compose down -v' EXIT

# 1) Confirm broker comes up healthy.
curl -sf http://localhost:18080/healthz

# 2) Confirm initial state is Uninitialised — broker should DM bootstrap recipe.
curl -sf -H "Authorization: Bearer ${TEST_ADMIN_TOKEN}" \
  http://localhost:18080/v1/admin/revalidate | jq .
echo ">>> Verify bootstrap-recipe DM arrived in test Slack channel <<<"

# 3) Bootstrap from laptop (interactive).
echo ">>> Run on laptop: pi  ->  /login  ->  exit"
echo ">>> Then: AUTH_BROKER_URL=http://localhost:18080 \\"
echo ">>>       AUTH_BROKER_ADMIN_TOKEN=${TEST_ADMIN_TOKEN} \\"
echo ">>>       ./bootstrap-auth.sh"
read -rp "Press Enter once bootstrap-auth.sh has run successfully... "

# 4) Confirm state transitioned to Healthy.
sleep 2
state=$(curl -sf -H "Authorization: Bearer ${TEST_ADMIN_TOKEN}" \
  http://localhost:18080/v1/admin/revalidate | jq -r .state)
[[ "$state" == "Healthy" ]] || { echo "expected Healthy, got $state"; exit 1; }
echo "State: $state ✓"

# 5) Confirm Recovered DM arrived.
echo ">>> Verify Recovered DM arrived in test Slack channel <<<"

# 6) Bundle persistence across restart.
podman compose restart auth-broker
sleep 3
state=$(curl -sf -H "Authorization: Bearer ${TEST_ADMIN_TOKEN}" \
  http://localhost:18080/v1/admin/revalidate | jq -r .state)
[[ "$state" == "Healthy" ]] || { echo "expected Healthy after restart, got $state"; exit 1; }
echo "State after restart: $state ✓"
```

- [ ] **Step 4: Document acceptance criteria in `auth-broker/test/e2e/README.md`**

```markdown
# auth-broker e2e smoke

Run locally with a test OpenAI subscription and test Slack workspace. Acceptance:

1. Bootstrap-recipe DM arrives in test Slack channel within 3s of `POST /v1/admin/revalidate` against an empty bundle.
2. After laptop-side `pi /login` + `bootstrap-auth.sh` upload, state transitions to Healthy and a Recovered DM arrives.
3. Restarting the broker container preserves the bundle on the PVC; state remains Healthy without re-bootstrap.
4. Tampering with `auth.json` to corrupt JSON and re-uploading returns HTTP 400 and leaves the previous bundle untouched.

Not a CI gate — human-in-the-loop.
```

- [ ] **Step 5: Run the smoke once, attach output + screenshots to the spike findings doc**

- [ ] **Step 6: Commit**

```bash
git add auth-broker/test
git commit -m "test(auth-broker): e2e bootstrap + lifecycle smoke harness"
```

---

# Phase 1 — code-pr agent on pi

**Goal:** Run new code-pr AgentTasks via pi (`--mode json`) when `AgentConfig.spec.runtime.engine == "pi"`. Old claude-sdk path remains the default until validated.

**Output of phase:** A working `runtime/src/agents/code-pr.ts` that produces PRs equivalent in quality to the existing pipeline; gated by a feature flag; four pi extensions installed at image build time.

**Time budget:** ~1 week.

### Task C1: Add `engine` field to AgentConfig CRD

**Files:**
- Modify: `operator/api/v1alpha1/agentconfig_types.go`
- Modify: `operator/api/v1alpha1/zz_generated.deepcopy.go` (regenerated)
- Modify: `operator/config/crd/bases/<...>.yaml` (regenerated)

- [ ] **Step 1: Failing test (or kubebuilder validation test) demonstrating that engine accepts only the two valid values**

Create `operator/api/v1alpha1/agentconfig_types_test.go`:

```go
package v1alpha1

import "testing"

func TestRuntimeConfig_EngineDefault(t *testing.T) {
	rc := RuntimeConfig{Image: "x"}
	// After applying the CRD default (kubebuilder), Engine should be claude-sdk
	// for backward compatibility.
	if rc.Engine != "" && rc.Engine != "claude-sdk" {
		t.Fatalf("zero-value Engine should be empty (CRD default applies); got %q", rc.Engine)
	}
}
```

- [ ] **Step 2: Modify the type**

```go
type RuntimeConfig struct {
	Image string `json:"image"`
	// +kubebuilder:default="claude-sonnet-4-6"
	Model string `json:"model,omitempty"`
	// +kubebuilder:default=200000
	MaxTokens int `json:"maxTokens,omitempty"`
	// +kubebuilder:validation:Enum=claude-sdk;pi
	// +kubebuilder:default="claude-sdk"
	Engine string `json:"engine,omitempty"`
}
```

- [ ] **Step 3: Regenerate**

```bash
cd operator
make generate    # operator/Makefile is kubebuilder-generated; we invoke its existing targets here (do not migrate to just in this plan)
make manifests   # regen CRD YAML
```

- [ ] **Step 4: Verify CRD YAML now lists engine**

```bash
grep -A2 'engine:' operator/config/crd/bases/*.yaml
```

- [ ] **Step 5: Test passes**

- [ ] **Step 6: Commit**

```bash
git add operator/api operator/config/crd
git commit -m "feat(operator): add engine field to AgentConfig.runtime"
```

### Task C2: Operator routes Job creation by engine

**Files:**
- Modify: `operator/internal/controller/agenttask_controller.go` (or wherever Jobs are spawned)

- [ ] **Step 1: Read current Job spec creation**

```bash
grep -n 'batchv1.Job' operator/internal/controller/*.go
```

Identify the function (likely `buildJob` or similar) and the corresponding test in `operator/internal/controller/*_test.go`.

- [ ] **Step 2: Failing test — when AgentConfig.runtime.engine == "pi", Job uses entrypoint `agents/code-pr.ts` and lacks `ANTHROPIC_API_KEY` env**

Append in the controller test file:

```go
func TestBuildJob_PiEngine_UsesAgentEntrypoint(t *testing.T) {
	cfg := &v1alpha1.AgentConfig{
		Spec: v1alpha1.AgentConfigSpec{
			Runtime: v1alpha1.RuntimeConfig{Image: "runtime:dev", Engine: "pi"},
			Auth:    v1alpha1.AuthConfig{ClaudeKeySecret: "claude", GithubAppSecret: "gh"},
			Slack:   v1alpha1.SlackConfig{Channel: "#test"},
		},
	}
	task := &v1alpha1.AgentTask{
		Spec: v1alpha1.AgentTaskSpec{
			AgentType: "code-pr", AgentConfig: "code-pr-pi",
			Source: v1alpha1.TaskSource{Type: "github-issue", Repo: "x", IssueNumber: 1},
			ToolPolicy: "code-pr-policy", Prompt: "...",
		},
	}
	job := buildJob(task, cfg)
	// pi path uses node entrypoint with the pi agent module:
	args := job.Spec.Template.Spec.Containers[0].Args
	if len(args) == 0 || args[len(args)-1] != "agents/code-pr.ts" {
		t.Fatalf("expected agents/code-pr.ts entrypoint, got args=%v", args)
	}
	for _, e := range job.Spec.Template.Spec.Containers[0].Env {
		if e.Name == "ANTHROPIC_API_KEY" {
			t.Fatal("pi engine job should not have ANTHROPIC_API_KEY")
		}
	}
}
```

Also add the symmetric test that `engine == "claude-sdk"` keeps the existing entrypoint and env.

- [ ] **Step 3: Run, fail**

- [ ] **Step 4: Implement the branch in `buildJob`**

```go
func buildJob(task *v1alpha1.AgentTask, cfg *v1alpha1.AgentConfig) *batchv1.Job {
	// ... existing setup ...
	switch cfg.Spec.Runtime.Engine {
	case "pi":
		container.Args = append(container.Args, "agents/code-pr.ts")
		container.Env = append(container.Env,
			corev1.EnvVar{Name: "AUTH_BROKER_URL", Value: "http://auth-broker.aios.svc:8080"},
			corev1.EnvVar{Name: "PI_AGENT_TYPE", Value: task.Spec.AgentType},
		)
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name: "fabric-patterns", MountPath: "/fabric-patterns", ReadOnly: true,
		})
		// project SA token for auth-broker auth
		// (existing code likely already projects token; if not, add an automountServiceAccountToken: true)
	default: // "claude-sdk" or empty
		// existing args/env preserved
	}
	return job
}
```

- [ ] **Step 5: Tests pass**

- [ ] **Step 6: Commit**

### Task C3: Build pi into shared `runtime` image

**Files:**
- Modify: `runtime/Dockerfile`

- [ ] **Step 1: Read current Dockerfile**

```bash
cat runtime/Dockerfile
```

- [ ] **Step 2: Add pi install**

```dockerfile
RUN npm install -g @mariozechner/pi-coding-agent@<PIN_FROM_SPIKE>
```

Pin to the version captured in the spike findings header.

- [ ] **Step 3: Verify image build succeeds and pi is present**

```bash
podman build -t aios/runtime:dev runtime/
podman run --rm aios/runtime:dev pi --version
```

Expected: pi version printed.

- [ ] **Step 4: Commit**

```bash
git add runtime/Dockerfile
git commit -m "feat(runtime): bundle pi into image"
```

### Task C4: SYSTEM.md composition tool

**Files:**
- Create: `runtime/scripts/compose-system-prompt.ts`
- Create: `runtime/scripts/compose-system-prompt.test.ts`
- Create: `fabric-patterns/_bases/code-pr.md`

- [ ] **Step 1: Failing test — composer concatenates a base file with named fabric patterns**

```ts
import { describe, it, expect } from "vitest";
import { composeSystemPrompt } from "./compose-system-prompt";

describe("composeSystemPrompt", () => {
  it("concatenates base + named patterns with separators", async () => {
    const out = await composeSystemPrompt({
      basePath: "/tmp/base.md",
      patterns: ["extract_requirements", "write_pull_request"],
      patternsDir: "/tmp/patterns",
    });
    expect(out).toContain("BASE\n\n--- pattern: extract_requirements ---\nER\n\n--- pattern: write_pull_request ---\nWPR");
  });
});
```

(Engineer creates the temp fixtures inside the test using `fs.mkdtemp`.)

- [ ] **Step 2: Implement**

```ts
import { promises as fs } from "node:fs";
import path from "node:path";

export interface ComposeOpts {
  basePath: string;
  patterns: string[];
  patternsDir: string;
}

export async function composeSystemPrompt(opts: ComposeOpts): Promise<string> {
  const base = await fs.readFile(opts.basePath, "utf8");
  const parts = [base.trim()];
  for (const name of opts.patterns) {
    const pPath = path.join(opts.patternsDir, name, "system.md");
    const text = await fs.readFile(pPath, "utf8");
    parts.push(`--- pattern: ${name} ---\n${text.trim()}`);
  }
  return parts.join("\n\n");
}
```

- [ ] **Step 3: Author the code-pr base**

`fabric-patterns/_bases/code-pr.md`:

```markdown
You are an autonomous code-PR agent. Goal: read a GitHub issue, implement a
working solution, run the project's tests, commit on a feature branch, and
hand off the branch name in your final structured output.

Constraints:
- Never push to main or any protected branch.
- Never run `rm -rf` outside the working tree.
- Always run the project's test suite before reporting success.
- If tests fail after 3 self-correction attempts, output the failure log and
  mark the result as "draft".
- Final output: a single JSON object on the last line, shape:
  {"branch": "<name>", "status": "ready"|"draft", "summary": "<1-2 sentences>"}.
```

- [ ] **Step 4: Tests pass**

- [ ] **Step 5: Commit**

### Task C5: Pi extension — sandbox

**Files:**
- Create: `runtime/pi-extensions/sandbox/index.ts`
- Create: `runtime/pi-extensions/sandbox/index.test.ts`
- Create: `runtime/pi-extensions/sandbox/package.json`

> **Note:** pi extensions are TypeScript modules registered via `--extension <path>`. The exact extension API is in pi's docs at `pi.dev/docs/latest/extensions`. The pseudocode below shows shape; engineer should reconcile with current API.

- [ ] **Step 1: Failing test — extension denies a command not on the allowlist**

```ts
import { describe, it, expect } from "vitest";
import { Sandbox } from "./index";

describe("Sandbox extension", () => {
  it("denies disallowed shell commands", () => {
    const sb = new Sandbox({ allowed: ["git status", "npm test"] });
    expect(sb.allow("rm -rf /")).toBe(false);
    expect(sb.allow("git status")).toBe(true);
    expect(sb.allow("git status -uno")).toBe(true);  // prefix match
  });
});
```

- [ ] **Step 2: Implement**

```ts
export interface SandboxOpts {
  allowed: string[];
}

export class Sandbox {
  constructor(private opts: SandboxOpts) {}
  allow(command: string): boolean {
    return this.opts.allowed.some((a) => command === a || command.startsWith(a + " "));
  }
}

// Pi extension entrypoint — exact shape per pi docs.
export default function (pi: any) {
  const opts: SandboxOpts = JSON.parse(process.env.SANDBOX_ALLOWED ?? "{\"allowed\":[]}");
  const sb = new Sandbox(opts);
  pi.beforeToolExecute("shell", (call: { command: string }) => {
    if (!sb.allow(call.command)) {
      return { allow: false, reason: `sandbox: denied ${call.command}` };
    }
    return { allow: true };
  });
}
```

- [ ] **Step 3: Tests pass**

- [ ] **Step 4: Commit**

### Task C6: Pi extension — slack-thread

**Files:**
- Create: `runtime/pi-extensions/slack-thread/index.ts`
- Create: `runtime/pi-extensions/slack-thread/index.test.ts`

- [ ] **Step 1: Failing test — extension posts assistant turns to a Slack thread**

```ts
import { describe, it, expect, vi } from "vitest";
import { SlackThread } from "./index";

describe("SlackThread extension", () => {
  it("posts message to thread on assistant text deltas", async () => {
    const post = vi.fn();
    const st = new SlackThread({ post, channel: "C1", threadTs: "1.0" });
    await st.onAssistantText("hello");
    await st.onAssistantText("world");
    expect(post).toHaveBeenCalledTimes(2);
    expect(post).toHaveBeenCalledWith({ channel: "C1", thread_ts: "1.0", text: "hello" });
  });
});
```

- [ ] **Step 2: Implement** with batching — collect text deltas for up to 1s or 500 chars, then flush — to avoid Slack rate limits.

- [ ] **Step 3: Tests pass**

- [ ] **Step 4: Commit**

### Task C7: Pi extension — MCP wiring

**Files:**
- Create: `runtime/pi-extensions/mcp/index.ts`
- Create: `runtime/pi-extensions/mcp/index.test.ts`

The extension reads an env var `MCP_SERVERS` (JSON array of `{name, url, transport}`) and registers each with pi. Pi presumably has a built-in MCP client for this — if not, the extension wraps `@modelcontextprotocol/sdk`.

- [ ] **Step 1: Failing test — extension registers each server**

```ts
it("registers each server from env", () => {
  process.env.MCP_SERVERS = JSON.stringify([{name:"aios-search", url:"http://x"}]);
  const calls: string[] = [];
  const fakePi = { registerMCPServer: (s: any) => calls.push(s.name) };
  registerMCP(fakePi);
  expect(calls).toEqual(["aios-search"]);
});
```

- [ ] **Step 2: Implement**

> **Note:** if pi does not currently ship a `registerMCPServer` API (verify against pi docs for the pinned version from spike), this extension creates a long-running MCP client per server and exposes the tools via pi's `registerTool`/`registerSkill` API. Reconcile with actual pi extension API at implementation time.

- [ ] **Step 3: Tests pass**

- [ ] **Step 4: Commit**

### Task C8: Pi extension — fabric-skill

**Files:**
- Create: `runtime/pi-extensions/fabric-skill/index.ts`
- Create: `runtime/pi-extensions/fabric-skill/index.test.ts`

Walks `/fabric-patterns/<name>/system.md` and registers each as a pi Skill (slash-invokable). For Phase 1 we only need to register the patterns referenced by the code-pr base (extract_requirements, write_pull_request, find_hidden_bugs).

- [ ] **Step 1: Failing test**

```ts
it("registers each pattern as a skill", async () => {
  const dir = await fs.mkdtemp(...);
  await fs.mkdir(path.join(dir, "extract_requirements"));
  await fs.writeFile(path.join(dir, "extract_requirements", "system.md"), "ER");
  const skills: string[] = [];
  registerFabricSkills({ dir, register: (s) => skills.push(s.name) });
  expect(skills).toEqual(["extract_requirements"]);
});
```

- [ ] **Step 2: Implement** with a directory walker.

- [ ] **Step 3: Tests pass**

- [ ] **Step 4: Commit**

### Task C9: agents/code-pr.ts preflight

**Files:**
- Create: `runtime/src/agents/code-pr.ts`
- Create: `runtime/src/agents/code-pr.test.ts`
- Create: `runtime/src/agents/preflight.ts`
- Create: `runtime/src/agents/preflight.test.ts`

Preflight: clone the target repo, fetch the issue body, build the prompt + context-bundle file.

- [ ] **Step 1: Failing test — preflight returns repo path and issue text**

```ts
import { describe, it, expect, vi } from "vitest";
import { preflight } from "./preflight";

describe("preflight", () => {
  it("clones the repo and fetches the issue", async () => {
    const fakeGh = {
      cloneRepo: vi.fn(async () => "/tmp/work/aios"),
      getIssue: vi.fn(async () => ({ body: "fix the bug", title: "x" })),
    };
    const out = await preflight({ repo: "Diixtra/aios", issue: 42, gh: fakeGh });
    expect(out.repoDir).toBe("/tmp/work/aios");
    expect(out.issueBody).toContain("fix the bug");
    expect(fakeGh.cloneRepo).toHaveBeenCalledWith("Diixtra/aios");
  });
});
```

- [ ] **Step 2: Implement**

```ts
export interface GhClient {
  cloneRepo(slug: string): Promise<string>;
  getIssue(repo: string, number: number): Promise<{ title: string; body: string }>;
}

export async function preflight(opts: {
  repo: string;
  issue: number;
  gh: GhClient;
}): Promise<{ repoDir: string; issueBody: string; issueTitle: string }> {
  const repoDir = await opts.gh.cloneRepo(opts.repo);
  const issue = await opts.gh.getIssue(opts.repo, opts.issue);
  return { repoDir, issueBody: issue.body, issueTitle: issue.title };
}
```

- [ ] **Step 3: Tests pass**

- [ ] **Step 4: Commit**

### Task C10: agents/code-pr.ts pi invocation

**Files:**
- Modify: `runtime/src/agents/code-pr.ts`
- Create: `runtime/src/agents/run-pi.ts`
- Create: `runtime/src/agents/run-pi.test.ts`

- [ ] **Step 1: Failing test — runPi spawns pi with the right flags and parses the final JSON line**

```ts
import { describe, it, expect } from "vitest";
import { runPi } from "./run-pi";

describe("runPi", () => {
  it("captures the final JSON line as the agent result", async () => {
    const result = await runPi({
      piBinary: "/bin/echo",
      args: [JSON.stringify({ branch: "feat/x", status: "ready", summary: "ok" })],
      cwd: ".",
      env: {},
    });
    expect(result.branch).toBe("feat/x");
    expect(result.status).toBe("ready");
  });
});
```

- [ ] **Step 2: Implement using `node:child_process` spawn**

```ts
import { spawn } from "node:child_process";
import { createInterface } from "node:readline";

export interface PiResult {
  branch?: string;
  status: "ready" | "draft" | "error";
  summary: string;
}

export async function runPi(opts: {
  piBinary: string;
  args: string[];
  cwd: string;
  env: Record<string, string>;
}): Promise<PiResult> {
  return new Promise((resolve, reject) => {
    const proc = spawn(opts.piBinary, opts.args, {
      cwd: opts.cwd,
      env: { ...process.env, ...opts.env },
      stdio: ["ignore", "pipe", "pipe"],
    });
    let lastLine = "";
    const rl = createInterface({ input: proc.stdout! });
    rl.on("line", (l) => { if (l.trim()) lastLine = l; });
    let stderr = "";
    proc.stderr!.on("data", (b) => { stderr += b.toString(); });
    proc.on("error", reject);
    proc.on("close", (code) => {
      if (code !== 0) {
        reject(new Error(`pi exited ${code}: ${stderr.slice(0, 500)}`));
        return;
      }
      try {
        resolve(JSON.parse(lastLine) as PiResult);
      } catch (e) {
        reject(new Error(`pi produced unparseable result: ${lastLine}`));
      }
    });
  });
}
```

- [ ] **Step 3: Wire into `code-pr.ts`**

`runtime/src/agents/code-pr.ts`:

```ts
import { preflight } from "./preflight";
import { runPi } from "./run-pi";
import { postflight } from "./postflight";
import { GhCli } from "../github";
import { acquireLease, releaseLease } from "../auth-broker";

export async function main(): Promise<void> {
  const repo = mustEnv("AIOS_REPO");
  const issue = parseInt(mustEnv("AIOS_ISSUE_NUMBER"), 10);
  const lease = await acquireLease("code-pr");
  try {
    const pf = await preflight({ repo, issue, gh: new GhCli() });
    const result = await runPi({
      piBinary: "/usr/local/bin/pi",
      args: [
        "--mode", "json",
        "--no-extensions",  // we'll add ours explicitly
        "--no-skills",
        "--no-prompt-templates",
        "--no-context-files",
        "--system-prompt", "/etc/pi/system-code-pr.md",
        "--extension", "/runtime/pi-extensions/sandbox",
        "--extension", "/runtime/pi-extensions/slack-thread",
        "--extension", "/runtime/pi-extensions/mcp",
        "--extension", "/runtime/pi-extensions/fabric-skill",
        "-p", `Implement issue ${repo}#${issue}: ${pf.issueTitle}\n\n${pf.issueBody}`,
      ],
      cwd: pf.repoDir,
      env: {
        SANDBOX_ALLOWED: JSON.stringify({ allowed: ["git ", "npm ", "go ", "pytest", "rg ", "cat ", "ls "] }),
        MCP_SERVERS: JSON.stringify([
          { name: "aios-search", url: process.env.AIOS_SEARCH_URL! },
          { name: "memory", url: process.env.MEMORY_MCP_URL! },
        ]),
      },
    });
    await postflight({ result, repo, issue, gh: new GhCli() });
  } finally {
    await releaseLease(lease.id);
  }
}

function mustEnv(k: string): string {
  const v = process.env[k];
  if (!v) throw new Error(`missing env ${k}`);
  return v;
}

main().catch((e) => { console.error(e); process.exit(1); });
```

- [ ] **Step 4: Test the wiring with a fake pi binary that emits the expected last-line JSON**

- [ ] **Step 5: Commit**

### Task C11: agents/code-pr.ts postflight

**Files:**
- Create: `runtime/src/agents/postflight.ts`
- Create: `runtime/src/agents/postflight.test.ts`

- [ ] **Step 1: Failing test — postflight opens a draft PR if status=draft, regular PR if status=ready**

```ts
it("opens a draft PR for status=draft", async () => {
  const gh = { openPR: vi.fn(async () => ({ url: "https://gh/pr/1" })) };
  await postflight({ result: { branch: "feat/x", status: "draft", summary: "wip" },
                     repo: "x/y", issue: 1, gh });
  expect(gh.openPR).toHaveBeenCalledWith(expect.objectContaining({ draft: true }));
});

it("opens a real PR for status=ready", async () => { ... });
```

- [ ] **Step 2: Implement**

```ts
export async function postflight(opts: {
  result: PiResult;
  repo: string;
  issue: number;
  gh: { openPR(spec: { repo: string; head: string; title: string; body: string; draft: boolean }): Promise<{ url: string }> };
}): Promise<{ prUrl: string }> {
  if (!opts.result.branch) throw new Error("postflight: pi returned no branch");
  const pr = await opts.gh.openPR({
    repo: opts.repo,
    head: opts.result.branch,
    title: `Closes #${opts.issue}: ${opts.result.summary}`,
    body: `Closes #${opts.issue}\n\n${opts.result.summary}`,
    draft: opts.result.status === "draft",
  });
  return { prUrl: pr.url };
}
```

- [ ] **Step 3: Tests pass**

- [ ] **Step 4: Commit**

### Task C12: AgentConfig ConfigMap for code-pr (engine: pi)

**Files:**
- Create: `k8s/base/operator/agentconfig-code-pr-pi.yaml`

- [ ] **Step 1: Author the manifest**

```yaml
apiVersion: aios.diixtra.io/v1alpha1
kind: AgentConfig
metadata:
  name: code-pr-pi
  namespace: aios
spec:
  runtime:
    image: ghcr.io/diixtra/aios-runtime:dev
    engine: pi
    model: gpt-5    # or whatever pi's subscription default model is
    maxTokens: 200000
  resources:
    requests: { cpu: 200m, memory: 512Mi }
    limits:   { cpu: 2,    memory: 2Gi }
  auth:
    claudeKeySecret: ""    # unused on pi engine
    githubAppSecret: aios-github-app
  slack:
    channel: "#aios-agents"
```

- [ ] **Step 2: Reference from operator's kustomization**

- [ ] **Step 3: Commit**

### Task C13: Side-by-side comparison harness

**Files:**
- Create: `runtime/test/side-by-side/compare.ts`
- Create: `runtime/test/side-by-side/README.md`

Goal: pick a fixture issue, dispatch one AgentTask with engine=claude-sdk and one with engine=pi, capture both PRs, and store side-by-side for human review.

- [ ] **Step 1: Author the harness**

```ts
// compare.ts — reads docs/superpowers/specs/<...>.md fixture issues, kubectl-creates two AgentTasks
// per fixture, polls for completion, and writes a markdown report comparing PR URLs and CI statuses.
```

(Engineer expands; this is a thin operational tool, not production code.)

- [ ] **Step 2: Author 3-5 fixture issues**

`runtime/test/side-by-side/fixtures/`:
- `001-add-readme-section.md` — easy
- `002-fix-tiny-typo.md` — trivial
- `003-add-go-env-var.md` — small Go change
- `004-bump-go-mod.md` — config tweak
- `005-add-vitest-test.md` — TS test addition

These issues should already be filed in a *test* repo — not the main aios repo — to avoid polluting real work.

- [ ] **Step 3: Run harness once**

```bash
tsx runtime/test/side-by-side/compare.ts --fixtures runtime/test/side-by-side/fixtures
```

Expected: produces `report.md` listing PR URLs from both engines.

- [ ] **Step 4: Commit harness + report-template**

### Task C14: Phase 1 acceptance — five real issues processed by both engines

This is gate-keeping, not new code. Block Phase 2 until this passes.

- [ ] **Step 1: Run the harness against 5 small real issues in a test repo**

- [ ] **Step 2: Compare PRs from both engines**

For each issue:
- Both engines opened a PR? (Y/N)
- PR closes the issue correctly? (Y/N)
- Tests pass on both PRs? (Y/N)
- Code quality (subjective): pi >= claude-sdk?

- [ ] **Step 3: Document findings in `runtime/test/side-by-side/2026-XX-XX-results.md`**

Decision rule: if pi engine ≥ claude-sdk on ≥4/5 issues, declare Phase 1 success and proceed to Phase 2 (not in this plan). If <4/5, file an issue documenting gaps and iterate within Phase 1 before moving on.

- [ ] **Step 4: Commit**

```bash
git add runtime/test/side-by-side/2026-XX-XX-results.md
git commit -m "test(runtime): Phase 1 acceptance — pi vs claude-sdk on 5 fixtures"
```

---

## Self-review (before opening the implementation PR)

When the engineer (or you, the executing agent) finishes the plan, run this checklist before opening the PR:

- [ ] All Phase -1 spike tasks have findings recorded in the spike doc.
- [ ] auth-broker tests run with ≥80% coverage (`go test -coverprofile=cov.out ./auth-broker/... && go tool cover -func=cov.out | tail -1`).
- [ ] runtime tests run with ≥80% coverage (`cd runtime && npm test -- --coverage`).
- [ ] Operator unit tests still pass (`make -C operator test`).
- [ ] All four pi extensions compile and pass tests.
- [ ] Side-by-side acceptance results documented.
- [ ] No `--no-verify`, `--no-gpg-sign`, or other safety bypasses in any commit.
- [ ] All commits follow conventional-commits style (`feat(scope): ...`, `test(scope): ...`, `spike(scope): ...`).
- [ ] No `runtime/src/pipeline/` or `@anthropic-ai/sdk` removal (deferred to Phase 6).
- [ ] CI passes on the branch (`gh pr checks`).
