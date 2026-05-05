# Long-Lived Mixed-Implementation Testnet Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Take xrpl-confluence from "ephemeral CI smoke harness" to "multi-day mixed-validator soak network capable of catching goXRPL panics, divergences, and consensus splits."

**Architecture:** Five sequential phases. Phase A merges nine queued fuzzer worktrees so subsequent phases work against a known post-M3c codebase. Phase B adds a crash-detection collector inside the existing sidecar. Phase C introduces a `MODE=soak` runner with persistent corpus + a host-bind `make soak` workflow. Phase D bumps the validator-key pool and rebalances default topology to 3 rippled + 2 goXRPL. Phase E exposes `fuzz_*` Prometheus metrics and adds a fuzzer panel to the dashboard.

**Tech Stack:** Go 1.24, Kurtosis/Starlark, Node 22 dashboard server, Docker (engine API for crash polling), `prometheus/client_golang`, `make`. `prometheus` + `grafana` are introduced in Phase E as additional Kurtosis services.

**Repository roots referenced throughout:**
- `xrpl-confluence/` — this repo (working dir for all `git`/`kurtosis`/`make` commands).
- `xrpl-confluence/.worktrees/fuzzer-m1` … `fuzzer-m3c/` — pre-existing worktrees holding the unmerged branches `fuzz/m1` … `fuzz/m3c`.
- Post-merge file paths (used in Phases B–E) live under `xrpl-confluence/sidecar/` and `xrpl-confluence/src/`.

**File Structure (post-merge ground truth, used by Phases B–E):**
- `sidecar/cmd/fuzz/main.go` — CLI entrypoint, env-var driven, dispatches on `MODE`.
- `sidecar/internal/fuzz/runners/realtime.go` — bounded fuzz loop (existing).
- `sidecar/internal/fuzz/runners/soak.go` — **new (Phase C)** unbounded loop reusing realtime helpers.
- `sidecar/internal/fuzz/crash/poller.go` — **new (Phase B)** Docker-API exit-code/log poller.
- `sidecar/internal/fuzz/crash/markers.go` — **new (Phase B)** crash-line classifier.
- `sidecar/internal/fuzz/metrics/metrics.go` — **new (Phase E)** Prometheus registry + `fuzz_*` collectors.
- `src/topology.star` — validator pool and config rendering (modified Phase D).
- `src/tests/soak.star` — **new (Phase C)** Kurtosis suite that brings up the network and the soak sidecar.
- `dashboard/static/{index.html,app.js,style.css}` — gain a Fuzzer panel (Phase E).
- `Makefile` — **new (Phase C)** with `soak`, `soak-down`, `soak-tail` targets.

---

## Phase A — Land the unmerged fuzzer work (`fuzz/m1` → `fuzz/m3c`)

Each of the nine branches has its own plan doc on the branch under `docs/plans/`. Phase A is sequential, one branch per task, because each successive milestone builds on the previous one's package layout.

### Task A1: Verify clean working tree and snapshot current `main`

**Files:**
- No edits. Pure verification.

- [ ] **Step 1: Confirm there are no uncommitted changes**

```bash
cd /Users/thomashussenet/Documents/project_goXRPL/xrpl-confluence
git status --porcelain
```

Expected: empty output. If anything appears, ask the user before proceeding.

- [ ] **Step 2: Snapshot the merge baseline**

```bash
git log --oneline -1 main
git tag -f pre-fuzzer-merge-baseline main
```

Expected: tag created on the current `main` HEAD (`1107cdb` at time of writing). The tag is local, used as a rollback target.

- [ ] **Step 3: Confirm all nine fuzzer branches exist on `origin`**

```bash
for b in fuzz/m1 fuzz/m2a fuzz/m2b fuzz/m2c fuzz/m2d fuzz/m2e fuzz/m3a fuzz/m3b fuzz/m3c; do
  git rev-parse --verify "origin/$b" >/dev/null && echo "OK $b" || echo "MISSING $b"
done
```

Expected: nine `OK` lines. Stop the phase if any branch is missing.

### Task A2: Merge `fuzz/m1`

**Files:**
- Modify (via merge): files added by the M1 branch under `sidecar/internal/fuzz/`, `sidecar/cmd/fuzz/`, `src/tests/fuzz.star`, `scripts/build-sidecar.sh`, `sidecar/Dockerfile`.

- [ ] **Step 1: Read the branch's plan doc**

Open `xrpl-confluence/.worktrees/fuzzer-m1/docs/plans/2026-04-22-fuzzer-m1.md` (the same plan that authored this branch). Skim Tasks 1–16 to confirm what landed.

- [ ] **Step 2: Diff the branch against `main`**

```bash
git fetch origin
git log --oneline main..origin/fuzz/m1
git diff --stat main..origin/fuzz/m1
```

Expected: ~16 commits and a stat summary showing `sidecar/internal/fuzz/{accounts,corpus,generator,runners}/...`, `sidecar/cmd/fuzz/main.go`, `src/tests/fuzz.star`, `main.star` updates. No changes outside `sidecar/`, `src/tests/`, `scripts/`, `Dockerfile`, `main.star`, `docs/plans/`.

- [ ] **Step 3: Run the branch's tests inside its worktree**

```bash
cd /Users/thomashussenet/Documents/project_goXRPL/xrpl-confluence/.worktrees/fuzzer-m1/sidecar
go test ./...
```

Expected: PASS. If any test fails, do **not** merge — open an issue and stop the phase. The `xrplgo_smoke_test.go` requires network only if it actually performs an outbound RPC; in the M1 plan it does not.

- [ ] **Step 4: Build the sidecar image from the branch**

```bash
cd /Users/thomashussenet/Documents/project_goXRPL/xrpl-confluence/.worktrees/fuzzer-m1
bash scripts/build-sidecar.sh
docker image inspect xrpl-confluence-sidecar:latest --format '{{.Id}}'
```

Expected: an image ID is printed (no inspect error).

- [ ] **Step 5: Smoke-run the fuzz suite end-to-end**

```bash
cd /Users/thomashussenet/Documents/project_goXRPL/xrpl-confluence/.worktrees/fuzzer-m1
kurtosis enclave rm -f fuzz-m1-merge >/dev/null 2>&1 || true
kurtosis run --enclave fuzz-m1-merge . '{"test_suite":"fuzz","goxrpl_count":1,"rippled_count":2}'
```

Expected: suite runs to completion in ≤ 10 minutes, final log line includes `txs_submitted >=` (any value ≥1). Tear down: `kurtosis enclave rm -f fuzz-m1-merge`.

- [ ] **Step 6: Fast-forward merge into `main`**

```bash
cd /Users/thomashussenet/Documents/project_goXRPL/xrpl-confluence
git checkout main
git merge --no-ff origin/fuzz/m1 -m "Merge fuzzer M1: skeleton + oracle layers 1+2"
```

Expected: merge commit created. If git reports conflicts (it shouldn't — `main` has not changed), abort with `git merge --abort` and re-plan.

- [ ] **Step 7: Push and verify CI passes**

```bash
git push origin main
```

Wait for the GitHub Actions / Kurtosis CI to complete (whichever is wired). If CI fails, revert with `git revert -m 1 HEAD` and stop the phase.

### Task A3: Merge `fuzz/m2a`

**Files:** changes under `sidecar/internal/fuzz/{accounts,generator}/` per the M2a plan doc on the branch.

- [ ] **Step 1: Read `docs/plans/...m2a.md` on the branch**

```bash
git show origin/fuzz/m2a:docs/plans/$(git ls-tree -r --name-only origin/fuzz/m2a | grep -E 'docs/plans/.*m2a.*\.md$' | head -1)
```

- [ ] **Step 2: Diff against current `main`**

```bash
git log --oneline main..origin/fuzz/m2a
git diff --stat main..origin/fuzz/m2a
```

Expected: only `sidecar/` paths and the new plan doc are touched.

- [ ] **Step 3: Run `go test ./...` on the branch**

```bash
cd /Users/thomashussenet/Documents/project_goXRPL/xrpl-confluence/.worktrees/fuzzer-m2a/sidecar
go test ./...
```

Expected: PASS.

- [ ] **Step 4: Smoke-run the fuzz suite from the branch**

```bash
cd /Users/thomashussenet/Documents/project_goXRPL/xrpl-confluence/.worktrees/fuzzer-m2a
bash scripts/build-sidecar.sh
kurtosis enclave rm -f fuzz-m2a-merge >/dev/null 2>&1 || true
kurtosis run --enclave fuzz-m2a-merge . '{"test_suite":"fuzz","goxrpl_count":1,"rippled_count":2}'
kurtosis enclave rm -f fuzz-m2a-merge
```

Expected: completes ≤ 10 min, no `divergences > 0` (a clean run).

- [ ] **Step 5: Merge into `main`**

```bash
cd /Users/thomashussenet/Documents/project_goXRPL/xrpl-confluence
git checkout main
git merge --no-ff origin/fuzz/m2a -m "Merge fuzzer M2a: SetupState polling + currency alignment"
git push origin main
```

Expected: clean merge, CI green.

### Task A4: Merge `fuzz/m2b`

**Files:** generic `Tx.Fields`/`SubmitTxJSON` plus AccountSet/TicketCreate/SetRegularKey builders.

- [ ] **Step 1: Read M2b plan doc on the branch.**
- [ ] **Step 2: `git log --oneline main..origin/fuzz/m2b` — confirm only `sidecar/internal/fuzz/generator/`, `sidecar/internal/rpcclient/client.go`, and the plan doc are touched.**
- [ ] **Step 3: `cd .worktrees/fuzzer-m2b/sidecar && go test ./...` — PASS.**
- [ ] **Step 4: Build + smoke-run the fuzz suite from the branch (same pattern as A3 step 4).**
- [ ] **Step 5: Merge into `main`:**

```bash
git checkout main
git merge --no-ff origin/fuzz/m2b -m "Merge fuzzer M2b: generic Tx.Fields + AccountSet/TicketCreate/SetRegularKey"
git push origin main
```

### Task A5: Merge `fuzz/m2c`

**Files:** `generator/mutator.go`, `MUTATION_RATE` env wiring in `cmd/fuzz/main.go` and `runners/realtime.go`.

- [ ] **Step 1: Read M2c plan doc.**
- [ ] **Step 2: Confirm branch diff is generator + cmd/fuzz only (`git diff --stat main..origin/fuzz/m2c`).**
- [ ] **Step 3: Run `go test ./...` on the branch — PASS.**
- [ ] **Step 4: Smoke-run with mutation enabled:**

```bash
cd /Users/thomashussenet/Documents/project_goXRPL/xrpl-confluence/.worktrees/fuzzer-m2c
bash scripts/build-sidecar.sh
kurtosis enclave rm -f fuzz-m2c-merge >/dev/null 2>&1 || true
kurtosis run --enclave fuzz-m2c-merge . '{"test_suite":"fuzz","goxrpl_count":1,"rippled_count":2,"fuzz_mutation_rate":"0.2"}'
kurtosis enclave rm -f fuzz-m2c-merge
```

Expected: stats JSON shows `txs_mutated > 0`. If the M2c `fuzz.star` doesn't yet expose `fuzz_mutation_rate` as an arg, set `MUTATION_RATE=0.2` directly in the suite Starlark for the smoke run, then revert.

- [ ] **Step 5: Merge:**

```bash
git checkout main
git merge --no-ff origin/fuzz/m2c -m "Merge fuzzer M2c: per-tx mutation modes"
git push origin main
```

### Task A6: Merge `fuzz/m2d`

**Files:** `oracle/tx_metadata.go`, `oracle/invariant_balance.go`, `rpcclient.Tx` returning raw `AffectedNodes`, runner wiring.

- [ ] **Step 1: Read M2d plan doc.**
- [ ] **Step 2: Confirm branch diff covers `sidecar/internal/oracle/` + `rpcclient/` + runner.**
- [ ] **Step 3: `go test ./...` PASS — including the new `tx_metadata_test.go` and `invariant_balance_test.go`.**
- [ ] **Step 4: Smoke-run; verify `stats.LedgersCompared > 0` and no spurious metadata divergences on a clean run.**
- [ ] **Step 5: Merge:**

```bash
git checkout main
git merge --no-ff origin/fuzz/m2d -m "Merge fuzzer M2d: oracle layers 3 (metadata) + 4 (XRP pool invariant)"
git push origin main
```

### Task A7: Merge `fuzz/m2e`

**Files:** `generator/tracker.go`, `generator/escrow_*.go`, `CandidateTx.CanBuild` eligibility gate, runner tracker feedback.

- [ ] **Step 1: Read M2e plan doc.**
- [ ] **Step 2: Diff confirms only generator + runner changes.**
- [ ] **Step 3: `go test ./...` PASS — escrow builder tests included.**
- [ ] **Step 4: Smoke-run; in the stats output check that `txs_succeeded` includes a few EscrowCreate then EscrowFinish/Cancel (grep the runner logs for `EscrowFinish` or `EscrowCancel`).**
- [ ] **Step 5: Merge:**

```bash
git checkout main
git merge --no-ff origin/fuzz/m2e -m "Merge fuzzer M2e: stateful tx eligibility + Escrow flow"
git push origin main
```

### Task A8: Merge `fuzz/m3a`

**Files:** `runners/replay.go`, `MODE=replay` in `cmd/fuzz/main.go`, mainnet RPC client + ledger iterator + address rewriter, `src/tests/replay.star` + `src/sidecar/replay.star`.

- [ ] **Step 1: Read M3a plan doc.**
- [ ] **Step 2: Confirm branch diff. M3a touches `sidecar/internal/fuzz/runners/replay*.go`, `sidecar/internal/mainnet/...` (or wherever the mainnet client lives), `cmd/fuzz/main.go`, and the new replay Starlark suites.**
- [ ] **Step 3: `go test ./...` PASS — replay unit tests do not require live mainnet (they should mock the RPC client; if any test reaches out to the network, gate it behind `-tags integration` before merging).**
- [ ] **Step 4: Skip live replay smoke run during merge (it depends on a real mainnet RPC and a long ledger range). Instead, run only the unit tests and a dry compile of the Starlark:**

```bash
cd /Users/thomashussenet/Documents/project_goXRPL/xrpl-confluence/.worktrees/fuzzer-m3a
kurtosis run --dry-run . '{"test_suite":"replay"}' || \
  kurtosis run --enclave m3a-syntax . '{"test_suite":"replay","replay_ledger_start":0,"replay_ledger_end":0}' &
sleep 5; kurtosis enclave rm -f m3a-syntax 2>/dev/null || true
```

Expected: no Starlark parse errors. (`--dry-run` may not be supported; the enclave-then-kill alternative is the fallback.)

- [ ] **Step 5: Merge:**

```bash
git checkout main
git merge --no-ff origin/fuzz/m3a -m "Merge fuzzer M3a: mainnet tx-shape replay runner"
git push origin main
```

### Task A9: Merge `fuzz/m3b`

**Files:** `corpus/runlog.go`, `runners/reproduce.go`, `MODE=reproduce`.

- [ ] **Step 1: Read M3b plan doc.**
- [ ] **Step 2: Diff covers corpus, runners, cmd/fuzz only.**
- [ ] **Step 3: `go test ./...` PASS, including the new `runlog_test.go` and `reproduce_test.go`.**
- [ ] **Step 4: No Kurtosis smoke required (reproduce uses an existing log; covered by unit tests).**
- [ ] **Step 5: Merge:**

```bash
git checkout main
git merge --no-ff origin/fuzz/m3b -m "Merge fuzzer M3b: run-log writer + reproduce mode"
git push origin main
```

### Task A10: Merge `fuzz/m3c`

**Files:** `corpus/signature.go`, `runners/shrink.go`, `MODE=shrink`, `oracle/wait.go`, `src/tests/shrink.star`, bisect driver script.

- [ ] **Step 1: Read M3c plan doc.**
- [ ] **Step 2: Diff covers corpus, oracle, runners, Starlark, and the bisect script under `scripts/`.**
- [ ] **Step 3: `go test ./...` PASS — `signature_test.go`, `shrink_test.go`, `wait_test.go`.**
- [ ] **Step 4: No live smoke (shrink uses a saved log + divergence file).**
- [ ] **Step 5: Merge:**

```bash
git checkout main
git merge --no-ff origin/fuzz/m3c -m "Merge fuzzer M3c: shrinker + bisect driver"
git push origin main
```

### Task A11: Delete legacy `trafficgen` (Post-M1 follow-up #1)

**Files:**
- Delete: `sidecar/cmd/trafficgen/`
- Modify: `src/sidecar/trafficgen.star` — delete file
- Modify: `src/tests/consensus.star` — remove `run_soak` + the `trafficgen` import (its responsibilities now live in `cmd/fuzz`)
- Modify: `sidecar/Dockerfile` — drop the `trafficgen` build stage if any
- Modify: `scripts/build-sidecar.sh` — drop `cmd/trafficgen` reference

- [ ] **Step 1: Verify trafficgen is unused after the merges**

```bash
git grep -nF "trafficgen" -- ':(exclude)docs/' ':(exclude).worktrees/'
```

Expected: hits only in the four files listed above (plus a possible legacy comment). If the fuzz suite or any test still imports it, do not delete.

- [ ] **Step 2: Delete the directory and references**

```bash
git rm -r sidecar/cmd/trafficgen
git rm src/sidecar/trafficgen.star
```

Edit `src/tests/consensus.star` to remove `trafficgen = import_module("../sidecar/trafficgen.star")` and the `run_soak` function (lines around 40–106 in the current file). Edit `src/tests/tests.star` to drop the `if suite == "soak":` branch — Phase C will reintroduce a soak suite under a new name.

Edit `sidecar/Dockerfile` and `scripts/build-sidecar.sh` to remove any `cmd/trafficgen` build/copy lines.

- [ ] **Step 3: Confirm the sidecar still builds and tests pass**

```bash
cd sidecar
go build ./...
go test ./...
cd ..
bash scripts/build-sidecar.sh
```

Expected: image rebuilds, tests pass.

- [ ] **Step 4: Smoke-run the fuzz suite**

```bash
kurtosis enclave rm -f post-trafficgen-smoke >/dev/null 2>&1 || true
kurtosis run --enclave post-trafficgen-smoke . '{"test_suite":"fuzz","goxrpl_count":1,"rippled_count":2}'
kurtosis enclave rm -f post-trafficgen-smoke
```

Expected: fuzz suite still runs to completion.

- [ ] **Step 5: Commit and push**

```bash
git add -A
git commit -m "confluence: retire trafficgen — superseded by cmd/fuzz"
git push origin main
```

---

## Phase B — Crash-detection collector

This phase adds a sidecar component that polls every container in the enclave each second, classifies non-zero exits, captures the last N log lines, and records a divergence-corpus entry tagged `kind: "crash"`. For goXRPL nodes it also sends `SIGQUIT` before death so a goroutine dump lands in the captured logs.

**Premise:** The existing `corpus.Recorder` API (`RecordDivergence(*Divergence)`) is already in use by `runners/realtime.go` (see Phase A). Phase B reuses it.

### Task B1: Crash-line classifier

**Files:**
- Create: `sidecar/internal/fuzz/crash/markers.go`
- Create: `sidecar/internal/fuzz/crash/markers_test.go`

- [ ] **Step 1: Write failing tests**

`sidecar/internal/fuzz/crash/markers_test.go`:

```go
package crash

import "testing"

func TestClassify_GoPanic(t *testing.T) {
	excerpt := []string{
		"random log line",
		"panic: runtime error: index out of range [5] with length 3",
		"goroutine 17 [running]:",
		"main.foo(...)",
	}
	c := Classify(excerpt)
	if c.Kind != "go_panic" {
		t.Fatalf("kind = %q, want go_panic", c.Kind)
	}
	if c.MarkerLine != 1 {
		t.Fatalf("marker line = %d, want 1", c.MarkerLine)
	}
}

func TestClassify_RippledAssert(t *testing.T) {
	excerpt := []string{
		"FATAL: assertion failed: foo == bar",
		"...",
	}
	c := Classify(excerpt)
	if c.Kind != "rippled_fatal" {
		t.Fatalf("kind = %q, want rippled_fatal", c.Kind)
	}
}

func TestClassify_Sigsegv(t *testing.T) {
	excerpt := []string{"some segfault: signal SIGSEGV: segmentation violation"}
	c := Classify(excerpt)
	if c.Kind != "sigsegv" {
		t.Fatalf("kind = %q, want sigsegv", c.Kind)
	}
}

func TestClassify_NoMarker(t *testing.T) {
	c := Classify([]string{"benign exit", "goodbye"})
	if c.Kind != "" {
		t.Fatalf("kind = %q, want empty", c.Kind)
	}
}
```

- [ ] **Step 2: Run the tests — expect FAIL**

```bash
cd sidecar
go test ./internal/fuzz/crash/...
```

Expected: build error or fail (file does not exist yet).

- [ ] **Step 3: Implement the classifier**

`sidecar/internal/fuzz/crash/markers.go`:

```go
// Package crash inspects container exits and log tails to identify
// process-level failures (panics, asserts, segfaults) that the oracle
// layers cannot see by themselves.
package crash

import "strings"

// Classification names a recognised failure pattern in the tail of a
// container's log. An empty Kind means no marker was found.
type Classification struct {
	Kind       string // "go_panic", "rippled_fatal", "sigsegv", "sigabrt", ""
	MarkerLine int    // index into the excerpt where the marker matched
}

var markers = []struct {
	kind     string
	patterns []string
}{
	{"go_panic", []string{"panic:", "fatal error:"}},
	{"rippled_fatal", []string{"FATAL:", "ASSERT", "assertion failed"}},
	{"sigsegv", []string{"SIGSEGV", "signal SIGSEGV", "segmentation violation"}},
	{"sigabrt", []string{"SIGABRT", "signal SIGABRT", "Aborted (core dumped)"}},
}

// Classify inspects an excerpt (one element per log line, oldest first)
// and returns the first matching classification.
func Classify(excerpt []string) Classification {
	for i, line := range excerpt {
		for _, m := range markers {
			for _, pat := range m.patterns {
				if strings.Contains(line, pat) {
					return Classification{Kind: m.kind, MarkerLine: i}
				}
			}
		}
	}
	return Classification{}
}
```

- [ ] **Step 4: Run the tests — expect PASS**

```bash
go test ./internal/fuzz/crash/...
```

Expected: PASS, all four cases.

- [ ] **Step 5: Commit**

```bash
git add sidecar/internal/fuzz/crash/markers.go sidecar/internal/fuzz/crash/markers_test.go
git commit -m "fuzzer: crash-line classifier (panic/fatal/sigsegv/sigabrt)"
```

### Task B2: Docker-API container poller

**Files:**
- Create: `sidecar/internal/fuzz/crash/poller.go`
- Create: `sidecar/internal/fuzz/crash/poller_test.go`
- Modify: `sidecar/go.mod` — add `github.com/docker/docker` and `github.com/docker/go-connections`

- [ ] **Step 1: Add the Docker SDK dependency**

```bash
cd sidecar
go get github.com/docker/docker/client@v25.0.5+incompatible
go get github.com/docker/docker/api/types
go get github.com/docker/docker/api/types/container
go mod tidy
```

Expected: `go.mod` updated. `+incompatible` is the published Docker SDK shape; if `go get` rejects it, fall back to the latest non-`+incompatible` tag and adjust imports.

- [ ] **Step 2: Write failing test (in-process fake Docker client)**

`sidecar/internal/fuzz/crash/poller_test.go`:

```go
package crash

import (
	"context"
	"testing"
	"time"
)

// fakeRuntime implements ContainerRuntime for unit testing.
type fakeRuntime struct {
	containers map[string]containerSnapshot
}

type containerSnapshot struct {
	exitCode int
	running  bool
	logs     []string
}

func (f *fakeRuntime) ListByLabel(ctx context.Context, key, val string) ([]string, error) {
	out := []string{}
	for name := range f.containers {
		out = append(out, name)
	}
	return out, nil
}

func (f *fakeRuntime) Inspect(ctx context.Context, name string) (running bool, exitCode int, err error) {
	s, ok := f.containers[name]
	if !ok {
		return false, 0, errNotFound
	}
	return s.running, s.exitCode, nil
}

func (f *fakeRuntime) TailLogs(ctx context.Context, name string, lines int) ([]string, error) {
	s, ok := f.containers[name]
	if !ok {
		return nil, errNotFound
	}
	return s.logs, nil
}

func (f *fakeRuntime) SendSignal(ctx context.Context, name, sig string) error { return nil }

func TestPoller_DetectsCrash(t *testing.T) {
	rt := &fakeRuntime{containers: map[string]containerSnapshot{
		"goxrpl-0": {running: false, exitCode: 2, logs: []string{
			"foo",
			"panic: runtime error: nil pointer dereference",
			"goroutine 1 [running]:",
		}},
	}}
	got := []*Event{}
	p := NewPoller(rt, "fuzzer.role", "node", 4)
	p.OnCrash = func(e *Event) { got = append(got, e) }

	if err := p.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}
	if got[0].Kind != "go_panic" || got[0].Container != "goxrpl-0" || got[0].ExitCode != 2 {
		t.Fatalf("unexpected event: %+v", got[0])
	}

	// Second tick must not re-fire the same crash.
	got = got[:0]
	if err := p.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("re-fired stale event: %+v", got)
	}
}

func TestPoller_IgnoresHealthyContainers(t *testing.T) {
	rt := &fakeRuntime{containers: map[string]containerSnapshot{
		"rippled-0": {running: true, exitCode: 0, logs: []string{"ok"}},
	}}
	p := NewPoller(rt, "fuzzer.role", "node", 4)
	called := false
	p.OnCrash = func(*Event) { called = true }
	if err := p.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("OnCrash fired for healthy container")
	}
	_ = time.Second
}
```

- [ ] **Step 3: Implement the poller skeleton (still failing — `errNotFound` undefined)**

`sidecar/internal/fuzz/crash/poller.go`:

```go
package crash

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// errNotFound is returned by ContainerRuntime when a container name is unknown.
var errNotFound = errors.New("container not found")

// ContainerRuntime is the minimal interface the poller needs. The Docker
// implementation lives in dockerruntime.go; tests inject a fake.
type ContainerRuntime interface {
	ListByLabel(ctx context.Context, key, val string) ([]string, error)
	Inspect(ctx context.Context, name string) (running bool, exitCode int, err error)
	TailLogs(ctx context.Context, name string, lines int) ([]string, error)
	SendSignal(ctx context.Context, name, sig string) error
}

// Event carries one detected crash to the OnCrash callback.
type Event struct {
	Container  string         `json:"container"`
	ExitCode   int            `json:"exit_code"`
	Kind       string         `json:"kind"` // from Classification
	MarkerLine int            `json:"marker_line"`
	LogTail    []string       `json:"log_tail"`
}

// Poller polls the runtime each Tick and fires OnCrash once per crashed
// container per crash event. Re-runs of the same container without a
// new exit do not re-fire.
type Poller struct {
	rt        ContainerRuntime
	labelKey  string
	labelVal  string
	tailLines int
	OnCrash   func(*Event)

	mu   sync.Mutex
	seen map[string]int // container -> last exitCode reported
}

// NewPoller constructs a Poller. It looks up containers labelled
// labelKey=labelVal and tails tailLines log lines on each detected crash.
func NewPoller(rt ContainerRuntime, labelKey, labelVal string, tailLines int) *Poller {
	return &Poller{
		rt:        rt,
		labelKey:  labelKey,
		labelVal:  labelVal,
		tailLines: tailLines,
		seen:      make(map[string]int),
	}
}

// Tick performs one round of inspection. Idempotent for already-reported crashes.
func (p *Poller) Tick(ctx context.Context) error {
	names, err := p.rt.ListByLabel(ctx, p.labelKey, p.labelVal)
	if err != nil {
		return fmt.Errorf("list: %w", err)
	}
	for _, name := range names {
		running, code, err := p.rt.Inspect(ctx, name)
		if err != nil {
			continue
		}
		if running || code == 0 {
			continue
		}
		p.mu.Lock()
		if last, ok := p.seen[name]; ok && last == code {
			p.mu.Unlock()
			continue
		}
		p.seen[name] = code
		p.mu.Unlock()

		tail, _ := p.rt.TailLogs(ctx, name, p.tailLines)
		cls := Classify(tail)
		if p.OnCrash != nil {
			p.OnCrash(&Event{
				Container:  name,
				ExitCode:   code,
				Kind:       cls.Kind,
				MarkerLine: cls.MarkerLine,
				LogTail:    tail,
			})
		}
	}
	return nil
}
```

- [ ] **Step 4: Run the tests — expect PASS**

```bash
go test ./internal/fuzz/crash/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add sidecar/go.mod sidecar/go.sum sidecar/internal/fuzz/crash/poller.go sidecar/internal/fuzz/crash/poller_test.go
git commit -m "fuzzer: crash poller — runtime-agnostic exit + log-tail collector"
```

### Task B3: Docker runtime implementation of `ContainerRuntime`

**Files:**
- Create: `sidecar/internal/fuzz/crash/dockerruntime.go`
- Modify: `sidecar/internal/fuzz/crash/poller_test.go` — no change yet; the Docker runtime is integration-tested manually in B5.

- [ ] **Step 1: Implement against the Docker SDK**

`sidecar/internal/fuzz/crash/dockerruntime.go`:

```go
package crash

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

// DockerRuntime implements ContainerRuntime against a local Docker daemon
// reachable via the host's UNIX socket. The sidecar mounts /var/run/docker.sock
// (see Phase B Task B5).
type DockerRuntime struct {
	cli *client.Client
}

// NewDockerRuntime dials the local Docker daemon. Caller closes via Close().
func NewDockerRuntime() (*DockerRuntime, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &DockerRuntime{cli: cli}, nil
}

// Close releases the underlying Docker client.
func (d *DockerRuntime) Close() error { return d.cli.Close() }

// ListByLabel returns container names (with leading "/" stripped) whose
// label matches key=val.
func (d *DockerRuntime) ListByLabel(ctx context.Context, key, val string) ([]string, error) {
	args := filters.NewArgs()
	args.Add("label", fmt.Sprintf("%s=%s", key, val))
	cs, err := d.cli.ContainerList(ctx, container.ListOptions{All: true, Filters: args})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		for _, n := range c.Names {
			out = append(out, strings.TrimPrefix(n, "/"))
		}
	}
	return out, nil
}

// Inspect returns the container's running flag and last exit code.
func (d *DockerRuntime) Inspect(ctx context.Context, name string) (bool, int, error) {
	info, err := d.cli.ContainerInspect(ctx, name)
	if err != nil {
		return false, 0, err
	}
	return info.State.Running, info.State.ExitCode, nil
}

// TailLogs returns up to lines log lines (combined stdout+stderr) ending at
// the current container tip.
func (d *DockerRuntime) TailLogs(ctx context.Context, name string, lines int) ([]string, error) {
	rdr, err := d.cli.ContainerLogs(ctx, name, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       fmt.Sprintf("%d", lines),
	})
	if err != nil {
		return nil, err
	}
	defer rdr.Close()
	var out []string
	scanner := bufio.NewScanner(rdr)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		// Docker multiplexed streams have an 8-byte header per frame. The
		// header bytes are non-printable; strip the leading 8 bytes if the
		// line starts with them.
		line := scanner.Text()
		if len(line) > 8 && line[0] < 0x20 {
			line = line[8:]
		}
		out = append(out, line)
	}
	return out, scanner.Err()
}

// SendSignal posts a kill signal (e.g. "QUIT", "TERM") to a container.
func (d *DockerRuntime) SendSignal(ctx context.Context, name, sig string) error {
	return d.cli.ContainerKill(ctx, name, sig)
}
```

- [ ] **Step 2: Compile-check**

```bash
cd sidecar
go build ./...
```

Expected: builds. (No new tests yet — wired up & smoke-tested in Task B5.)

- [ ] **Step 3: Commit**

```bash
git add sidecar/internal/fuzz/crash/dockerruntime.go
git commit -m "fuzzer: Docker runtime adapter for crash poller"
```

### Task B4: Hook poller into the fuzz runner

**Files:**
- Modify: `sidecar/internal/fuzz/runners/realtime.go` — start a poller goroutine, route events through the existing `corpus.Recorder`.
- Modify: `sidecar/internal/fuzz/runners/realtime_test.go` — add a test that wires a fake runtime and observes a `kind: "crash"` divergence.
- Modify: `sidecar/cmd/fuzz/main.go` — accept env var `CRASH_LABEL_KEY` (default `fuzzer.role`) and `CRASH_LABEL_VAL` (default `node`); skip poller if `CRASH_LABEL_VAL` is empty (allows unit tests to disable).

- [ ] **Step 1: Add a method on `runners.Config`**

In `sidecar/internal/fuzz/runners/realtime.go`, extend `Config`:

```go
type Config struct {
	NodeURLs     []string
	SubmitURL    string
	Seed         uint64
	AccountN     int
	TxCount      int
	CorpusDir    string
	BatchClose   time.Duration
	SkipFund     bool
	SkipSetup    bool
	MutationRate float64
	// CrashRuntime, when non-nil, is polled once per BatchClose tick and
	// crash events are recorded as divergences (kind="crash"). Nil disables.
	CrashRuntime  crash.ContainerRuntime
	CrashLabelKey string // e.g. "fuzzer.role"
	CrashLabelVal string // e.g. "node"
	CrashTailLines int   // log lines to capture on crash (default 200)
}
```

Add the import: `"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/crash"`.

- [ ] **Step 2: Start the poller in `Run`**

Just below the existing `rec := corpus.NewRecorder(...)` line, add:

```go
var poller *crash.Poller
if cfg.CrashRuntime != nil && cfg.CrashLabelVal != "" {
	tail := cfg.CrashTailLines
	if tail == 0 {
		tail = 200
	}
	poller = crash.NewPoller(cfg.CrashRuntime, cfg.CrashLabelKey, cfg.CrashLabelVal, tail)
	poller.OnCrash = func(e *crash.Event) {
		atomic.AddInt64(&stats.Divergences, 1)
		_ = rec.RecordDivergence(&corpus.Divergence{
			Kind:        "crash",
			Description: fmt.Sprintf("%s exited %d (%s)", e.Container, e.ExitCode, e.Kind),
			Details: map[string]any{
				"container":   e.Container,
				"exit_code":   e.ExitCode,
				"crash_kind":  e.Kind,
				"marker_line": e.MarkerLine,
				"log_tail":    e.LogTail,
			},
		})
	}
}
```

In the existing periodic block (the `if cfg.BatchClose > 0 && i%10 == 9 {` branch), call:

```go
if poller != nil {
	_ = poller.Tick(ctx)
}
```

**Note:** `stats` is declared further down in the current file. Move the `var stats Stats; stats.Seed = cfg.Seed` declaration up so the poller closure captures it. If that ordering change is fragile, an equivalent fix is to declare `stats` before the poller block and use `&stats` directly in the closure.

- [ ] **Step 3: Add the unit test**

Append to `sidecar/internal/fuzz/runners/realtime_test.go`:

```go
func TestRun_RecordsCrashAsDivergence(t *testing.T) {
	tmp := t.TempDir()
	rt := &fakeCrashRuntime{exits: map[string]struct {
		code int
		logs []string
	}{
		"goxrpl-0": {code: 2, logs: []string{"panic: nil pointer"}},
	}}
	cfg := Config{
		NodeURLs:       []string{"http://node-a", "http://node-b"},
		SubmitURL:      "http://node-a",
		Seed:           42,
		AccountN:       1,
		TxCount:        0,
		CorpusDir:      tmp,
		SkipFund:       true,
		SkipSetup:      true,
		CrashRuntime:   rt,
		CrashLabelKey:  "fuzzer.role",
		CrashLabelVal:  "node",
		CrashTailLines: 4,
		BatchClose:     1 * time.Millisecond,
	}
	// ... drive Run for one tick using a context with deadline 50ms; assert
	// /output/corpus/divergences/ contains an entry whose JSON has
	// "kind":"crash" and "container":"goxrpl-0".
}
```

The `fakeCrashRuntime` is implemented inline (mirroring the fake from B2) in the same test file. The full test body is mechanical; if the existing `realtime_test.go` already mocks `oracle.Node`, follow that pattern.

- [ ] **Step 4: Run tests — expect PASS**

```bash
go test ./internal/fuzz/runners/...
```

Expected: PASS, including the new crash test.

- [ ] **Step 5: Wire the env vars into `cmd/fuzz/main.go`**

Inside `loadConfig()` in `sidecar/cmd/fuzz/main.go`, after the existing env reads, add:

```go
if val := os.Getenv("CRASH_LABEL_VAL"); val != "" {
	rt, err := crash.NewDockerRuntime()
	if err != nil {
		log.Printf("fuzz: crash poller disabled — docker dial failed: %v", err)
	} else {
		cfg.CrashRuntime = rt
		cfg.CrashLabelKey = envDefault("CRASH_LABEL_KEY", "fuzzer.role")
		cfg.CrashLabelVal = val
		if n, err := strconv.Atoi(envDefault("CRASH_TAIL_LINES", "200")); err == nil {
			cfg.CrashTailLines = n
		}
	}
}
```

Add the import `"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/crash"`.

- [ ] **Step 6: Commit**

```bash
git add sidecar/internal/fuzz/runners/realtime.go sidecar/internal/fuzz/runners/realtime_test.go sidecar/cmd/fuzz/main.go
git commit -m "fuzzer: poll containers each tick, record crashes as divergences"
```

### Task B5: Mount Docker socket and label nodes in Kurtosis

**Files:**
- Modify: `src/rippled/rippled.star` — add `labels = {"fuzzer.role": "node"}` on the rippled service.
- Modify: `src/goxrpl/goxrpl.star` — same label, plus `cmd_args` change so the goXRPL process inherits PID 1 (or wrap in `tini`) so `SIGQUIT` reaches Go runtime — confirm via local test.
- Modify: `src/sidecar/fuzz.star` — mount `/var/run/docker.sock` and pass `CRASH_LABEL_VAL=node`.

- [ ] **Step 1: Add labels to node services**

In `src/rippled/rippled.star`, locate the `ServiceConfig(...)` for rippled and add (or append to existing):

```python
labels = {"fuzzer.role": "node"},
```

Same edit in `src/goxrpl/goxrpl.star`.

- [ ] **Step 2: Mount the Docker socket on the sidecar**

In `src/sidecar/fuzz.star`, inside the `ServiceConfig(...)`:

```python
files = {
    "/output": Directory(persistent_key="fuzz-output"),
    "/var/run/docker.sock": "/var/run/docker.sock",  # host bind for crash poller
},
env_vars = env_vars | {
    "CRASH_LABEL_KEY": "fuzzer.role",
    "CRASH_LABEL_VAL": "node",
    "CRASH_TAIL_LINES": "200",
},
```

If Kurtosis Starlark does not allow host-path binds in `files`, swap to `user_services` configuration with a `volumes` field, or mount via `service_config = ServiceConfig(... privileged_mode = True ...)` plus a host-path mount. Adjust to whatever the local Kurtosis version exposes; the contract is "the sidecar can read `/var/run/docker.sock`."

- [ ] **Step 3: Verify locally with the smoke fuzz suite**

```bash
bash scripts/build-sidecar.sh
kurtosis enclave rm -f crash-smoke >/dev/null 2>&1 || true
kurtosis run --enclave crash-smoke . '{"test_suite":"fuzz","goxrpl_count":1,"rippled_count":2}'
kurtosis service logs crash-smoke fuzz | grep -E 'crash poller|disabled'
```

Expected: log line `fuzz: crash poller …` appears, **not** `crash poller disabled`. No `panic` or `Aborted` on a clean run, so no crash events are emitted (this is correct).

- [ ] **Step 4: Force a crash and observe a recorded divergence**

```bash
docker kill --signal=SEGV "$(docker ps --filter name=goxrpl-0 --format '{{.ID}}')"
sleep 10
kurtosis service exec crash-smoke fuzz 'ls /output/corpus/divergences/'
kurtosis service exec crash-smoke fuzz \
  'grep -l "\"kind\":\"crash\"" /output/corpus/divergences/*.json | head -1 | xargs cat'
```

Expected: a divergence file exists with `"kind":"crash"`, `"crash_kind":"sigsegv"` (or `"go_panic"` depending on whether goXRPL traps the signal), and a non-empty `log_tail`.

- [ ] **Step 5: Tear down and commit**

```bash
kurtosis enclave rm -f crash-smoke
git add src/rippled/rippled.star src/goxrpl/goxrpl.star src/sidecar/fuzz.star
git commit -m "confluence: label nodes + mount docker.sock for crash poller"
git push origin main
```

### Task B6: SIGQUIT for goroutine dump on goXRPL hang/panic

**Files:**
- Modify: `sidecar/internal/fuzz/crash/poller.go` — extend the watch path to send `SIGQUIT` to a goXRPL container that becomes unresponsive (no validated-seq advance for N ticks) before its eventual exit, so a goroutine dump lands in the logs we tail.

- [ ] **Step 1: Add a "hang detector" to the poller**

Append to `sidecar/internal/fuzz/crash/poller.go`:

```go
// HangDetector tracks per-container liveness signals (e.g. validated_ledger.seq)
// and asks the runtime to send SIGQUIT to a hung container so a Go goroutine
// dump or rippled stack trace is written before the eventual exit.
type HangDetector struct {
	StaleTicks int                          // how many consecutive same-value ticks count as hung
	Liveness   func(ctx context.Context, name string) (signal int64, err error)
	Match      func(name string) bool       // only call SIGQUIT on matching containers (e.g. goXRPL)
	last       map[string]int64
	stale      map[string]int
	fired      map[string]bool
}

// NewHangDetector returns a detector that triggers after staleTicks consecutive
// unchanged liveness samples.
func NewHangDetector(staleTicks int) *HangDetector {
	return &HangDetector{
		StaleTicks: staleTicks,
		last:       map[string]int64{},
		stale:      map[string]int{},
		fired:      map[string]bool{},
	}
}

// Step samples liveness for one container and returns true if SIGQUIT should
// fire (and has not yet fired this run).
func (h *HangDetector) Step(ctx context.Context, name string) bool {
	if h.fired[name] || h.Match == nil || !h.Match(name) || h.Liveness == nil {
		return false
	}
	v, err := h.Liveness(ctx, name)
	if err != nil {
		return false
	}
	if v == h.last[name] {
		h.stale[name]++
	} else {
		h.last[name] = v
		h.stale[name] = 0
	}
	if h.stale[name] >= h.StaleTicks {
		h.fired[name] = true
		return true
	}
	return false
}
```

- [ ] **Step 2: Wire the detector into the runner**

In `sidecar/internal/fuzz/runners/realtime.go`, at the periodic-tick block, before `poller.Tick`:

```go
if hang != nil {
	for _, n := range nodes {
		if hang.Step(ctx, n.Name) {
			log.Printf("realtime: container %s appears hung — SIGQUIT", n.Name)
			_ = cfg.CrashRuntime.SendSignal(ctx, n.Name, "QUIT")
		}
	}
}
```

Construct `hang` near the poller, with `Liveness` calling `n.Client.ServerInfo()` and returning `info.Validated.Seq`, and `Match` returning `strings.HasPrefix(name, "goxrpl-")`.

- [ ] **Step 3: Unit test**

Add to `sidecar/internal/fuzz/crash/poller_test.go`:

```go
func TestHangDetector_FiresAfterStaleTicks(t *testing.T) {
	h := NewHangDetector(3)
	h.Match = func(name string) bool { return name == "goxrpl-0" }
	h.Liveness = func(_ context.Context, _ string) (int64, error) { return 100, nil }
	for i := 0; i < 2; i++ {
		if h.Step(context.Background(), "goxrpl-0") {
			t.Fatalf("fired too early at i=%d", i)
		}
	}
	if !h.Step(context.Background(), "goxrpl-0") {
		t.Fatal("expected fire on third stale tick")
	}
	if h.Step(context.Background(), "goxrpl-0") {
		t.Fatal("must not fire twice")
	}
}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
cd sidecar
go test ./internal/fuzz/crash/... ./internal/fuzz/runners/...
```

- [ ] **Step 5: Smoke-run with a deliberately hung goXRPL**

(Manual: replace the goXRPL image with a `sleep infinity` shim or send `SIGSTOP` to it.) Confirm a `kind: "crash"` divergence is recorded with `crash_kind: "go_panic"` and a `goroutine` block in the `log_tail`.

- [ ] **Step 6: Commit**

```bash
git add sidecar/internal/fuzz/crash/ sidecar/internal/fuzz/runners/realtime.go
git commit -m "fuzzer: hang detector — SIGQUIT a stuck goXRPL before it exits"
git push origin main
```

---

## Phase C — Long-lived (`MODE=soak`) runner with persistent corpus

The realtime runner caps at `TxCount`. The soak runner must run indefinitely until externally cancelled, rotate funded accounts so XRP is recycled, and write its corpus to a path that survives Kurtosis enclave teardown.

### Task C1: Soak runner skeleton (in-process, no Kurtosis yet)

**Files:**
- Create: `sidecar/internal/fuzz/runners/soak.go`
- Create: `sidecar/internal/fuzz/runners/soak_test.go`

- [ ] **Step 1: Failing test**

`sidecar/internal/fuzz/runners/soak_test.go`:

```go
package runners

import (
	"context"
	"testing"
	"time"
)

// TestSoakRun_StopsOnContextCancel boots a soak run with a tight context and
// verifies it returns cleanly with stats covering the work it managed to do.
func TestSoakRun_StopsOnContextCancel(t *testing.T) {
	tmp := t.TempDir()
	cfg := SoakConfig{
		Config: Config{
			NodeURLs:  []string{"http://stub-a", "http://stub-b"},
			SubmitURL: "http://stub-a",
			Seed:      7,
			AccountN:  2,
			CorpusDir: tmp,
			SkipFund:  true,
			SkipSetup: true,
		},
		TxRate:        1,                       // 1 tx/s
		RotateEvery:   100,                     // accounts rotate after 100 successes
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	stats, err := SoakRun(ctx, cfg)
	if err != nil && err != context.DeadlineExceeded {
		t.Fatalf("SoakRun: %v", err)
	}
	if stats == nil {
		t.Fatal("nil stats")
	}
}
```

- [ ] **Step 2: Implement `SoakConfig` + `SoakRun`**

`sidecar/internal/fuzz/runners/soak.go`:

```go
package runners

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/accounts"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/generator"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/oracle"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// SoakConfig extends the bounded Config with soak-specific knobs.
type SoakConfig struct {
	Config
	TxRate      float64 // submissions per second; 0 = uncapped
	RotateEvery int64   // tx successes between account-pool tier rotations
}

// SoakRun runs an unbounded fuzz loop until ctx is cancelled. It reuses the
// realtime helpers (pool, generator, oracle, recorder) but never returns
// based on a tx count.
func SoakRun(ctx context.Context, cfg SoakConfig) (*Stats, error) {
	if len(cfg.NodeURLs) < 2 {
		return nil, fmt.Errorf("need >= 2 NodeURLs")
	}
	submit := rpcclient.New(cfg.SubmitURL)
	nodes := make([]oracle.Node, len(cfg.NodeURLs))
	for i, u := range cfg.NodeURLs {
		nodes[i] = oracle.Node{Name: nodeName(u), Client: rpcclient.New(u)}
	}
	orc := oracle.New(nodes)
	rec := corpus.NewRecorder(cfg.CorpusDir, cfg.Seed)
	txLog, err := corpus.NewRunLog(cfg.CorpusDir, cfg.Seed)
	if err != nil {
		return nil, fmt.Errorf("run log: %w", err)
	}
	defer txLog.Close()

	pool, err := accounts.NewPool(cfg.Seed, cfg.AccountN)
	if err != nil {
		return nil, err
	}
	rng := corpus.NewRNG(cfg.Seed)

	if !cfg.SkipFund {
		if err := accounts.FundFromGenesis(submit, pool, 10_000_000_000); err != nil {
			return nil, fmt.Errorf("fund: %w", err)
		}
		time.Sleep(5 * time.Second)
	}
	if !cfg.SkipSetup {
		if err := accounts.SetupState(submit, pool); err != nil {
			return nil, fmt.Errorf("setup state: %w", err)
		}
	}

	enabled, err := generator.DiscoverEnabledAmendments(submit)
	if err != nil {
		return nil, err
	}
	gen := generator.New(pool)

	var stats Stats
	stats.Seed = cfg.Seed

	// Pacing.
	var ticker *time.Ticker
	if cfg.TxRate > 0 {
		ticker = time.NewTicker(time.Duration(float64(time.Second) / cfg.TxRate))
		defer ticker.Stop()
	}

	step := 0
	for {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return &stats, nil
			}
			return &stats, err
		}
		if ticker != nil {
			select {
			case <-ctx.Done():
				return &stats, nil
			case <-ticker.C:
			}
		}

		tx, err := gen.PickTx(rng.Rand(), enabled)
		if err != nil {
			atomic.AddInt64(&stats.TxsFailed, 1)
			continue
		}
		if cfg.MutationRate > 0 {
			if mutated, did := gen.Mutator().Maybe(rng.Rand(), tx, cfg.MutationRate); did {
				tx = mutated
				atomic.AddInt64(&stats.TxsMutated, 1)
			}
		}
		atomic.AddInt64(&stats.TxsSubmitted, 1)
		res, err := submit.SubmitTxJSON(tx.Secret, tx.Fields)
		if err != nil || (res.EngineResult != "tesSUCCESS" && res.EngineResult != "terQUEUED") {
			atomic.AddInt64(&stats.TxsFailed, 1)
			continue
		}
		atomic.AddInt64(&stats.TxsSucceeded, 1)
		_ = txLog.Append(&corpus.RunLogEntry{
			Step: step, TxType: tx.TransactionType(), Fields: tx.Fields, Secret: tx.Secret,
			Result: res.EngineResult, TxHash: res.TxHash,
		})
		step++

		if res.TxHash != "" {
			if cmp := orc.CompareTxResult(ctx, res.TxHash); !cmp.Agreed {
				atomic.AddInt64(&stats.Divergences, 1)
				_ = rec.RecordDivergence(&corpus.Divergence{
					Kind: "tx_result", Description: fmt.Sprintf("tx %s disagreed", res.TxHash),
					Details: map[string]any{"tx_hash": res.TxHash, "node_results": cmp.NodeResults},
				})
			}
			if meta := orc.CompareTxMetadata(ctx, res.TxHash); !meta.Agreed {
				atomic.AddInt64(&stats.Divergences, 1)
				_ = rec.RecordDivergence(&corpus.Divergence{
					Kind: "metadata", Description: fmt.Sprintf("tx %s metadata diverged", res.TxHash),
					Details: map[string]any{"tx_hash": res.TxHash, "node_meta": meta.NodeMeta},
				})
			}
		}
		if cfg.RotateEvery > 0 && atomic.LoadInt64(&stats.TxsSucceeded)%cfg.RotateEvery == 0 {
			log.Printf("soak: rotating account tiers at %d successes", stats.TxsSucceeded)
			if err := accounts.RotateTiers(submit, pool, rng.Rand()); err != nil {
				log.Printf("soak: rotate: %v", err)
			}
		}
	}
}
```

The function `accounts.RotateTiers` is introduced in C2 below; until that lands, leave a `// TODO C2` and have the soak loop ignore the rotation knob. Tests still pass.

- [ ] **Step 3: Run tests — expect PASS**

```bash
cd sidecar
go test ./internal/fuzz/runners/...
```

- [ ] **Step 4: Commit**

```bash
git add sidecar/internal/fuzz/runners/soak.go sidecar/internal/fuzz/runners/soak_test.go
git commit -m "fuzzer: soak runner — unbounded tx loop, ctx-cancellable"
```

### Task C2: Account-tier rotation

**Files:**
- Modify: `sidecar/internal/fuzz/accounts/pool.go` — add `RotateTiers` (or a new file `rotate.go` if cleaner).
- Modify: `sidecar/internal/fuzz/accounts/pool_test.go`
- Modify: `sidecar/internal/fuzz/runners/soak.go` — uncomment the rotation call once the function exists.

- [ ] **Step 1: Failing test**

In `sidecar/internal/fuzz/accounts/pool_test.go`, append:

```go
func TestRotateTiers_RecyclesXRP(t *testing.T) {
	t.Skip("M1 ships rich-only; rotation is a no-op stub here. " +
		"Replace this skip when M2/M3 add multiple tiers.")
}
```

(Phase C explicitly does not introduce new account tiers — that's a future M4 follow-up. The rotation hook is added so the runner has a clean place to call it later, and so a long-running soak can `Payment` excess XRP back to a treasury account at intervals to avoid stagnant pool state.)

- [ ] **Step 2: Implement `RotateTiers` as XRP-recycle**

Add to `sidecar/internal/fuzz/accounts/pool.go`:

```go
import "math/rand/v2"

// RotateTiers walks the pool and submits a no-op self-Payment of 1 drop from
// each rich account back to itself, refreshing the account's last-used
// timestamp on every node and exercising the sequence-advance path. Future
// versions will move XRP between tiers; M1's pool is rich-only so this is a
// pacing tick.
func RotateTiers(submit *rpcclient.Client, pool *Pool, rng *rand.Rand) error {
	for _, w := range pool.All() {
		_, err := submit.SubmitTxJSON(w.Seed, map[string]any{
			"TransactionType": "AccountSet",
			"Account":         w.ClassicAddress,
		})
		if err != nil {
			return err
		}
	}
	return nil
}
```

(The import path for `rpcclient` is already in the file — confirm; if not, add it.)

- [ ] **Step 3: Wire the call back in `soak.go`** — replace the `// TODO C2` block with `accounts.RotateTiers(submit, pool, rng.Rand())`.

- [ ] **Step 4: Run tests — expect PASS**

```bash
cd sidecar
go test ./internal/fuzz/accounts/... ./internal/fuzz/runners/...
```

- [ ] **Step 5: Commit**

```bash
git add sidecar/internal/fuzz/accounts/pool.go sidecar/internal/fuzz/accounts/pool_test.go sidecar/internal/fuzz/runners/soak.go
git commit -m "fuzzer: tier-rotation hook (no-op for rich-only pool)"
```

### Task C3: Wire `MODE=soak` in `cmd/fuzz/main.go`

**Files:**
- Modify: `sidecar/cmd/fuzz/main.go`

- [ ] **Step 1: Add a soak case to the dispatch switch**

Inside `switch mode { ... }`, before `default`:

```go
case "soak":
	cfg, err := loadSoakConfig()
	if err != nil {
		log.Fatalf("soak config: %v", err)
	}
	log.Printf("soak: seed=%d nodes=%d submit=%s rate=%.2f rotate_every=%d",
		cfg.Seed, len(cfg.NodeURLs), cfg.SubmitURL, cfg.TxRate, cfg.RotateEvery)
	stats, err := runners.SoakRun(ctx, *cfg)
	if err != nil {
		log.Fatalf("soak: %v", err)
	}
	statsMu.Lock()
	currentStats = stats
	statsMu.Unlock()
	blob, _ := json.MarshalIndent(stats, "", "  ")
	log.Printf("soak: done\n%s", blob)
```

- [ ] **Step 2: Add `loadSoakConfig`**

Below `loadConfig()`:

```go
func loadSoakConfig() (*runners.SoakConfig, error) {
	base, err := loadConfig()
	if err != nil {
		return nil, err
	}
	rate, _ := strconv.ParseFloat(envDefault("TX_RATE", "0"), 64)
	rotate, _ := strconv.ParseInt(envDefault("ROTATE_EVERY", "1000"), 10, 64)
	return &runners.SoakConfig{
		Config:      *base,
		TxRate:      rate,
		RotateEvery: rotate,
	}, nil
}
```

- [ ] **Step 3: Update the package doc comment to list `MODE=soak`** with the new env vars (`TX_RATE`, `ROTATE_EVERY`).

- [ ] **Step 4: Build and unit-test**

```bash
cd sidecar
go build ./...
go test ./...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add sidecar/cmd/fuzz/main.go
git commit -m "fuzzer: cmd/fuzz MODE=soak wiring"
```

### Task C4: Persistent corpus — host-bind on the Kurtosis sidecar

**Files:**
- Create: `src/tests/soak.star`
- Modify: `src/tests/tests.star` — add a `soak` suite branch.
- Modify: `src/sidecar/fuzz.star` — accept a `corpus_host_path` option that switches `/output` from a Kurtosis-managed `Directory` to a host-bind.
- Modify: `main.star` — extend the `test_suite` docstring to list `"soak"`.

- [ ] **Step 1: Add a `soak` Starlark suite**

`src/tests/soak.star`:

```python
"""Soak suite — unbounded fuzz loop on the existing topology."""

helpers = import_module("../helpers/rpc.star")
fuzz_sidecar = import_module("../sidecar/fuzz.star")


def run(plan, nodes, args = {}):
    """Bring up the fuzz sidecar in MODE=soak.

    Args:
        plan: Kurtosis plan object.
        nodes: List of node descriptors.
        args: Optional dict with keys:
            - tx_rate: float, submissions per second (default 0 = uncapped).
            - rotate_every: int, successes between rotations (default 1000).
            - mutation_rate: float, per-tx mutation probability (default 0.0).
            - accounts: int, account-pool size (default 50).
            - corpus_host_path: str, host directory for /output bind (required
              for persistence; if empty, /output is enclave-only).

    Returns:
        Soak service reference. Caller is responsible for tailing logs.
    """
    plan.print("Waiting for network to be live (closed_seq >= 3)...")
    for n in nodes:
        helpers.wait_for_ledger_seq(plan, n, 3, timeout = "120s")

    rippled_nodes = [n for n in nodes if n["type"] == "rippled"]
    submit = rippled_nodes[0] if len(rippled_nodes) > 0 else nodes[0]

    return fuzz_sidecar.launch_soak(
        plan,
        all_nodes = nodes,
        submit_node = submit,
        tx_rate = args.get("tx_rate", 0),
        rotate_every = args.get("rotate_every", 1000),
        mutation_rate = args.get("mutation_rate", 0.0),
        accounts = args.get("accounts", 50),
        corpus_host_path = args.get("corpus_host_path", ""),
    )
```

- [ ] **Step 2: Implement `launch_soak` in `src/sidecar/fuzz.star`**

Add (mirroring the existing `launch` function):

```python
def launch_soak(plan, all_nodes, submit_node, tx_rate, rotate_every, mutation_rate, accounts, corpus_host_path):
    node_addrs = ",".join(["http://{}:5005".format(n["name"]) for n in all_nodes])
    submit_url = "http://{}:5005".format(submit_node["name"])

    files = {}
    if corpus_host_path != "":
        # Host-bind /output so the corpus survives `kurtosis enclave rm`.
        # Kurtosis 1.x exposes host paths via `Directory(host_path = ...)`.
        files["/output"] = Directory(host_path = corpus_host_path)
    else:
        files["/output"] = Directory(persistent_key = "fuzz-soak-output")

    files["/var/run/docker.sock"] = "/var/run/docker.sock"

    return plan.add_service(
        name = "fuzz-soak",
        config = ServiceConfig(
            image = "xrpl-confluence-sidecar:latest",
            ports = {
                "results": PortSpec(number = 8081, transport_protocol = "TCP", application_protocol = "http"),
            },
            files = files,
            cmd = ["/out/fuzz"],
            env_vars = {
                "MODE": "soak",
                "NODES": node_addrs,
                "SUBMIT_URL": submit_url,
                "ACCOUNTS": str(accounts),
                "TX_RATE": str(tx_rate),
                "ROTATE_EVERY": str(rotate_every),
                "MUTATION_RATE": str(mutation_rate),
                "CRASH_LABEL_KEY": "fuzzer.role",
                "CRASH_LABEL_VAL": "node",
                "CORPUS_DIR": "/output/corpus",
            },
        ),
    )
```

If the running Kurtosis version names host binds differently (e.g. `Volume(host_path=)`), adjust to the local API; the contract is "the sidecar's `/output` is backed by the host path the operator passed in."

- [ ] **Step 3: Route the suite in `tests.star`**

In `src/tests/tests.star`, add an import and dispatch:

```python
soak = import_module("./soak.star")
# ... in run():
if suite == "soak":
    plan.print("=== Running soak ===")
    results["soak"] = soak.run(plan, nodes, args.get("soak_args", {}))
    return results
```

- [ ] **Step 4: Update `main.star` docstring**

Edit the `test_suite` docstring to list `"soak"` and add a top-level `soak_args` arg.

- [ ] **Step 5: Smoke-launch (10 seconds, then teardown)**

```bash
mkdir -p /tmp/fuzz-soak-corpus
kurtosis enclave rm -f soak-smoke >/dev/null 2>&1 || true
kurtosis run --enclave soak-smoke . \
  '{"test_suite":"soak","goxrpl_count":1,"rippled_count":2,"soak_args":{"corpus_host_path":"/tmp/fuzz-soak-corpus","tx_rate":2,"accounts":5}}' &
KPID=$!
sleep 60
kill $KPID
kurtosis service logs soak-smoke fuzz-soak | tail -20
ls /tmp/fuzz-soak-corpus/corpus/
kurtosis enclave rm -f soak-smoke
```

Expected: log shows `soak: rotating account tiers` at least once, `/tmp/fuzz-soak-corpus/corpus/` contains entries, divergences directory exists (likely empty on a clean run).

- [ ] **Step 6: Commit**

```bash
git add src/tests/soak.star src/tests/tests.star src/sidecar/fuzz.star main.star
git commit -m "confluence: soak suite — unbounded fuzz with host-bind corpus"
git push origin main
```

### Task C5: `make soak` workflow

**Files:**
- Create: `Makefile` (top-level of `xrpl-confluence/`)

- [ ] **Step 1: Create the Makefile**

`xrpl-confluence/Makefile`:

```make
ENCLAVE      ?= xrpl-soak
GOXRPL_COUNT ?= 2
RIPPLED_COUNT?= 3
TX_RATE      ?= 5
ACCOUNTS     ?= 50
ROTATE_EVERY ?= 1000
MUTATION_RATE?= 0.05
CORPUS       ?= $(PWD)/.soak-corpus

.PHONY: soak soak-down soak-tail soak-status

soak:
	@mkdir -p $(CORPUS)
	@bash scripts/build-sidecar.sh
	kurtosis enclave rm -f $(ENCLAVE) >/dev/null 2>&1 || true
	kurtosis run --enclave $(ENCLAVE) . '{"test_suite":"soak","goxrpl_count":$(GOXRPL_COUNT),"rippled_count":$(RIPPLED_COUNT),"soak_args":{"corpus_host_path":"$(CORPUS)","tx_rate":$(TX_RATE),"accounts":$(ACCOUNTS),"rotate_every":$(ROTATE_EVERY),"mutation_rate":$(MUTATION_RATE)}}'
	@echo "Dashboard: http://$$(kurtosis service inspect $(ENCLAVE) dashboard | awk '/IP Address/ {print $$3; exit}'):8080"
	@echo "Corpus:    $(CORPUS)"

soak-down:
	kurtosis enclave rm -f $(ENCLAVE)

soak-tail:
	kurtosis service logs -f $(ENCLAVE) fuzz-soak

soak-status:
	kurtosis enclave inspect $(ENCLAVE)
```

- [ ] **Step 2: Smoke-test the make targets**

```bash
cd /Users/thomashussenet/Documents/project_goXRPL/xrpl-confluence
make soak GOXRPL_COUNT=1 RIPPLED_COUNT=2 TX_RATE=2 ACCOUNTS=5 &
SOAK_BG=$!
sleep 30
make soak-tail | head -20
make soak-down
wait $SOAK_BG 2>/dev/null
ls .soak-corpus/corpus/
```

Expected: enclave comes up, fuzz-soak service appears, log tail shows tx submissions, host-bound corpus has entries, `make soak-down` cleans the enclave.

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "confluence: make soak / soak-down / soak-tail workflow"
git push origin main
```

---

## Phase D — Topology rebalance

Bumping the validator-key pool to ten and changing the default mix from 4+1 to 3+2 lets a goXRPL state bug actually fork the chain.

### Task D1: Generate five additional validator keypairs

**Files:**
- Modify: `scripts/keygen/main.go` — already exists; rerun.
- Modify: `src/topology.star` — append five entries to `VALIDATOR_KEYS`.

- [ ] **Step 1: Run the keygen tool**

```bash
cd /Users/thomashussenet/Documents/project_goXRPL/xrpl-confluence/scripts/keygen
go run .
```

Expected: prints 5 new `{seed, pubkey}` JSON or Starlark-shaped lines. If the existing keygen prints only one per run, run it five times. If the format differs, copy-paste-format into the Starlark `{"seed":"...", "pubkey":"..."}` shape.

- [ ] **Step 2: Append to `VALIDATOR_KEYS`**

In `src/topology.star`, extend the list to ten entries:

```python
VALIDATOR_KEYS = [
    {"seed": "sneWFZcEqA8TUA5BmJ38xsqaR7dFb", "pubkey": "n9LXMXFTeVL6o9fxdFHfeVZWf6YzWCBzt7YyeK1HV7wZ4ZFRNgUV"},
    {"seed": "snjbY5o3g4zK8dtotD6wjdNV3i96r", "pubkey": "n9KTo9UAFTV2XPZG8oUbuwNBhvwVF2fkyxz9jE88iGhJVoV3Sxy4"},
    {"seed": "sn8KuG4fs84rowCsqTuz6AtqEkmJ7", "pubkey": "n9KVs96MmgjXmok33PNEr29xbRAfvqvw1HqQYGsWE9zBdJMYJ9Pc"},
    {"seed": "sha6zPXQHAEwVk1qEREAxZPqy7h5Z", "pubkey": "n9KRLEqrFzXi5yK3XE6NUhcFx8XLHWZg3SczPb8doFCiryPSmvfr"},
    {"seed": "snPRr5dyXnYYZ4idydxHxhm2qnohc", "pubkey": "n9Jjt6fFpdTzms5tpYAf2iFyQwXNZWrQgwtrbwQEvFWQN4kfRFPb"},
    # appended in Phase D Task D1:
    {"seed": "<NEW_SEED_6>", "pubkey": "<NEW_PUBKEY_6>"},
    {"seed": "<NEW_SEED_7>", "pubkey": "<NEW_PUBKEY_7>"},
    {"seed": "<NEW_SEED_8>", "pubkey": "<NEW_PUBKEY_8>"},
    {"seed": "<NEW_SEED_9>", "pubkey": "<NEW_PUBKEY_9>"},
    {"seed": "<NEW_SEED_10>", "pubkey": "<NEW_PUBKEY_10>"},
]
```

Replace the `<NEW_*>` placeholders with values produced by Step 1.

- [ ] **Step 3: Commit**

```bash
git add src/topology.star
git commit -m "topology: bump validator pool to 10 keys"
```

### Task D2: Default soak topology to 3 rippled + 2 goXRPL

**Files:**
- Modify: `main.star` — change `DEFAULT_GOXRPL_COUNT` to 1 still for legacy `all`/`propagation`/`sync`/`consensus`, but inject soak-specific defaults via `soak.star`.
- Modify: `src/tests/soak.star` — clamp `goxrpl_count`/`rippled_count` defaults via the suite, not by changing the global default.

- [ ] **Step 1: Edit `src/tests/soak.star` to assert minimum mix**

Inside `run(plan, nodes, args)` add at the top:

```python
goxrpl_nodes = [n for n in nodes if n["type"] == "goxrpl"]
rippled_nodes = [n for n in nodes if n["type"] == "rippled"]
if len(goxrpl_nodes) < 2 or len(rippled_nodes) < 2:
    fail("soak suite requires >= 2 goXRPL and >= 2 rippled validators (got {} goxrpl, {} rippled)".format(
        len(goxrpl_nodes), len(rippled_nodes),
    ))
```

This is a guardrail so a soak run cannot accidentally launch with the legacy 4+1 default.

- [ ] **Step 2: Update the `make soak` defaults**

The Makefile already defaults to `RIPPLED_COUNT=3 GOXRPL_COUNT=2`. Confirm.

- [ ] **Step 3: Smoke-run the soak suite at the new mix**

```bash
make soak TX_RATE=3 ACCOUNTS=10 &
SOAK_BG=$!
sleep 90
make soak-tail | grep -E 'soak:|Stats' | tail -20
make soak-down
wait $SOAK_BG 2>/dev/null
```

Expected: 3 rippled + 2 goXRPL containers, all reach validated seq > 5, soak runner submits txs and produces `Stats` log lines.

- [ ] **Step 4: Verify rippled nodes accept goXRPL validator pubkeys**

```bash
# from any rippled node
docker exec $(docker ps --filter name=rippled-0 --format '{{.ID}}') /opt/ripple/bin/rippled --conf /etc/rippled/rippled-0.cfg server_info | jq '.result.info.validator_list'
```

Expected: `validator_list` count == 5 (the network total). Validation quorum is met.

- [ ] **Step 5: Commit**

```bash
git add src/tests/soak.star Makefile
git commit -m "soak: enforce 3+2 minimum mix; default make soak topology"
git push origin main
```

---

## Phase E — Prometheus metrics + dashboard panel

The fuzzer exposes counters/gauges per the design doc. The dashboard adds a Fuzzer panel that reads `/metrics` directly (no Prometheus server needed for the basic panel — just `fetch('/metrics')` and parse). For long-term storage / alerting, an optional Prometheus + Grafana service can be enabled.

### Task E1: Metrics package and `/metrics` endpoint

**Files:**
- Create: `sidecar/internal/fuzz/metrics/metrics.go`
- Create: `sidecar/internal/fuzz/metrics/metrics_test.go`
- Modify: `sidecar/cmd/fuzz/main.go` — register and serve `/metrics`.
- Modify: `sidecar/go.mod` — add `github.com/prometheus/client_golang`.

- [ ] **Step 1: Add Prometheus client**

```bash
cd sidecar
go get github.com/prometheus/client_golang/prometheus
go get github.com/prometheus/client_golang/prometheus/promhttp
go mod tidy
```

- [ ] **Step 2: Failing test**

`sidecar/internal/fuzz/metrics/metrics_test.go`:

```go
package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegistry_ExposesFuzzCounters(t *testing.T) {
	r := New()
	r.TxsSubmitted.WithLabelValues("Payment", "valid").Inc()
	r.Divergences.WithLabelValues("tx_result").Inc()
	r.Crashes.WithLabelValues("goxrpl-0", "go_panic").Inc()

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body := readAll(t, resp.Body)

	for _, want := range []string{
		`fuzz_txs_submitted_total{mode="valid",tx_type="Payment"} 1`,
		`fuzz_divergences_total{layer="tx_result"} 1`,
		`fuzz_crashes_total{impl="go_panic",node="goxrpl-0"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing line %q in /metrics output", want)
		}
	}
}
```

(`readAll` is a tiny helper using `io.ReadAll` — write it inline in the test file.)

- [ ] **Step 3: Implement the registry**

`sidecar/internal/fuzz/metrics/metrics.go`:

```go
// Package metrics exposes the fuzz_* Prometheus surface called out in the
// xrpl-confluence fuzzer design.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry wraps a private *prometheus.Registry so the fuzzer's metrics
// don't leak into any default registries.
type Registry struct {
	reg *prometheus.Registry

	TxsSubmitted *prometheus.CounterVec   // labels: tx_type, mode (valid|mutated|random)
	TxsApplied   *prometheus.CounterVec   // labels: tx_type, result (e.g. tesSUCCESS)
	Divergences  *prometheus.CounterVec   // labels: layer (state_hash|tx_result|metadata|invariant|crash)
	Crashes      *prometheus.CounterVec   // labels: node, impl
	AccountsActive prometheus.Gauge
	CorpusSize     prometheus.Gauge
	CurrentSeed    prometheus.Gauge
	OracleLatency  *prometheus.HistogramVec // labels: layer
	CloseDuration  prometheus.Histogram
}

// New constructs and registers all collectors on a fresh registry.
func New() *Registry {
	r := &Registry{reg: prometheus.NewRegistry()}

	r.TxsSubmitted = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "fuzz_txs_submitted_total"},
		[]string{"tx_type", "mode"},
	)
	r.TxsApplied = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "fuzz_txs_applied_total"},
		[]string{"tx_type", "result"},
	)
	r.Divergences = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "fuzz_divergences_total"},
		[]string{"layer"},
	)
	r.Crashes = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "fuzz_crashes_total"},
		[]string{"node", "impl"},
	)
	r.AccountsActive = prometheus.NewGauge(prometheus.GaugeOpts{Name: "fuzz_accounts_active"})
	r.CorpusSize = prometheus.NewGauge(prometheus.GaugeOpts{Name: "fuzz_corpus_size"})
	r.CurrentSeed = prometheus.NewGauge(prometheus.GaugeOpts{Name: "fuzz_current_seed"})
	r.OracleLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Name: "fuzz_oracle_latency_seconds", Buckets: prometheus.DefBuckets},
		[]string{"layer"},
	)
	r.CloseDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{Name: "fuzz_close_duration_seconds", Buckets: prometheus.DefBuckets},
	)

	for _, c := range []prometheus.Collector{
		r.TxsSubmitted, r.TxsApplied, r.Divergences, r.Crashes,
		r.AccountsActive, r.CorpusSize, r.CurrentSeed,
		r.OracleLatency, r.CloseDuration,
	} {
		r.reg.MustRegister(c)
	}
	return r
}

// Handler returns an http.Handler serving the fuzz_* metrics in
// Prometheus text exposition format.
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.reg, promhttp.HandlerOpts{})
}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
go test ./internal/fuzz/metrics/...
```

- [ ] **Step 5: Wire into `cmd/fuzz/main.go`**

In the existing `serveHTTP` block, register the metrics handler:

```go
mreg := metrics.New()
http.Handle("/metrics", mreg.Handler())
```

Pass `mreg` to the runners by extending `Config` with a `Metrics *metrics.Registry` (nil-tolerant). In `runners.Run` and `runners.SoakRun`, on each tx submission and divergence record, increment the matching counter / set the matching gauge. Example diff in `realtime.go`:

```go
if cfg.Metrics != nil {
    cfg.Metrics.TxsSubmitted.WithLabelValues(tx.TransactionType(), txMode).Inc()
}
```

where `txMode` is `"valid"`, `"mutated"`, or `"random"` (deduce from generator/mutator path).

- [ ] **Step 6: Build, test, smoke**

```bash
cd sidecar
go build ./...
go test ./...
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add sidecar/go.mod sidecar/go.sum sidecar/internal/fuzz/metrics sidecar/cmd/fuzz/main.go sidecar/internal/fuzz/runners
git commit -m "fuzzer: prometheus fuzz_* metrics + /metrics endpoint"
```

### Task E2: Dashboard panel reading `/metrics`

**Files:**
- Modify: `dashboard/server.js` — add an `/api/fuzz` proxy endpoint that fetches the sidecar's `/metrics`, parses the `fuzz_*` lines, and returns JSON.
- Modify: `dashboard/static/index.html` — add a `<section id="fuzz-panel">` block.
- Modify: `dashboard/static/app.js` — poll `/api/fuzz` every 5 s and render counters/gauges.
- Modify: `dashboard/static/style.css` — basic styling for the panel.
- Modify: `src/dashboard/dashboard.star` — pass the sidecar URL into the dashboard config.

- [ ] **Step 1: Extend `config.json` with `fuzz_metrics_url`**

In `src/dashboard/dashboard.star`, add to `config_content`:

```python
fuzz_metrics_url = "http://fuzz-soak:8081/metrics" if has_soak else ""
config_content = '{{"nodes":[{}],"poll_interval_ms":5000,"fuzz_metrics_url":"{}"}}'.format(",".join(nodes_json), fuzz_metrics_url)
```

`has_soak` is true when the suite is `soak` — the dashboard launcher receives that information from `main.star`. If the launcher signature can't accept it without a wider refactor, hard-code `"http://fuzz-soak:8081/metrics"` and let the panel render "no fuzzer running" when the URL 404s.

- [ ] **Step 2: Add the dashboard server route**

In `dashboard/server.js`, alongside the existing `/api/nodes` handler:

```js
if (req.url === "/api/fuzz") {
  if (!config.fuzz_metrics_url) {
    res.writeHead(204);
    res.end();
    return;
  }
  fetchText(config.fuzz_metrics_url, 3000)
    .then((txt) => {
      const out = parseFuzzMetrics(txt);
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify(out));
    })
    .catch((e) => {
      res.writeHead(502, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ error: e.message }));
    });
  return;
}
```

`fetchText` is a small helper using `http.request` (mirror the existing `rpcCall`). `parseFuzzMetrics` is a few lines of regex over the Prometheus text format that emits an object like:

```json
{
  "txs_submitted_total": 1234,
  "txs_applied_total": 1180,
  "divergences_total_by_layer": {"tx_result": 0, "metadata": 0, "state_hash": 0, "crash": 1},
  "crashes_total": 1,
  "accounts_active": 50,
  "current_seed": 4321
}
```

- [ ] **Step 3: Add the panel markup**

In `dashboard/static/index.html`, add inside the main grid:

```html
<section id="fuzz-panel">
  <h2>Fuzzer</h2>
  <div class="fuzz-grid">
    <div class="kpi"><div class="label">Submitted</div><div id="fuzz-submitted" class="value">—</div></div>
    <div class="kpi"><div class="label">Applied</div><div id="fuzz-applied" class="value">—</div></div>
    <div class="kpi"><div class="label">Divergences</div><div id="fuzz-divergences" class="value">—</div></div>
    <div class="kpi"><div class="label">Crashes</div><div id="fuzz-crashes" class="value">—</div></div>
    <div class="kpi"><div class="label">Seed</div><div id="fuzz-seed" class="value">—</div></div>
  </div>
  <table id="fuzz-by-layer">
    <thead><tr><th>Layer</th><th>Divergences</th></tr></thead>
    <tbody></tbody>
  </table>
</section>
```

- [ ] **Step 4: Add the polling JS**

In `dashboard/static/app.js`, add:

```js
async function pollFuzz() {
  try {
    const r = await fetch("/api/fuzz");
    if (r.status === 204) return;
    if (!r.ok) return;
    const data = await r.json();
    document.getElementById("fuzz-submitted").textContent = data.txs_submitted_total ?? "—";
    document.getElementById("fuzz-applied").textContent = data.txs_applied_total ?? "—";
    document.getElementById("fuzz-divergences").textContent = data.divergences_total ?? "—";
    document.getElementById("fuzz-crashes").textContent = data.crashes_total ?? "—";
    document.getElementById("fuzz-seed").textContent = data.current_seed ?? "—";
    const tbody = document.querySelector("#fuzz-by-layer tbody");
    tbody.innerHTML = "";
    for (const [layer, count] of Object.entries(data.divergences_total_by_layer ?? {})) {
      const tr = document.createElement("tr");
      tr.innerHTML = `<td>${layer}</td><td>${count}</td>`;
      tbody.appendChild(tr);
    }
  } catch (_) {
    // panel stays at "—" until the sidecar comes up.
  }
}
setInterval(pollFuzz, 5000);
pollFuzz();
```

- [ ] **Step 5: CSS**

`dashboard/static/style.css` (append):

```css
#fuzz-panel { padding: 1rem; }
#fuzz-panel .fuzz-grid { display: grid; grid-template-columns: repeat(5, 1fr); gap: 0.75rem; }
#fuzz-panel .kpi { background: #18202c; padding: 0.5rem 0.75rem; border-radius: 4px; }
#fuzz-panel .kpi .label { font-size: 0.75rem; color: #8aa; }
#fuzz-panel .kpi .value { font-size: 1.4rem; color: #d8e8f5; }
#fuzz-by-layer { width: 100%; margin-top: 0.5rem; border-collapse: collapse; }
#fuzz-by-layer th, #fuzz-by-layer td { padding: 0.25rem 0.5rem; border-bottom: 1px solid #233; text-align: left; }
```

- [ ] **Step 6: Smoke-test the panel**

```bash
make soak TX_RATE=3 ACCOUNTS=5 &
SOAK_BG=$!
sleep 60
DASH_IP=$(kurtosis service inspect xrpl-soak dashboard | awk '/IP Address/ {print $3; exit}')
curl -s http://$DASH_IP:8080/api/fuzz | jq .
make soak-down
wait $SOAK_BG 2>/dev/null
```

Expected: `/api/fuzz` returns a JSON object with non-zero `txs_submitted_total`. Open the dashboard in a browser and confirm the panel renders.

- [ ] **Step 7: Commit**

```bash
git add dashboard/server.js dashboard/static/index.html dashboard/static/app.js dashboard/static/style.css src/dashboard/dashboard.star
git commit -m "dashboard: fuzzer panel — pulls /metrics, renders KPIs + per-layer divergences"
git push origin main
```

### Task E3: (Optional) Prometheus + Grafana sidecars for long-term storage

**Files:**
- Create: `src/sidecar/prometheus.star`
- Create: `src/sidecar/grafana.star`
- Modify: `src/tests/soak.star` — when `args["enable_observability"]` is true, launch both.

This is a deliberately small task because the dashboard panel covers day-to-day visibility; Prometheus is for week-long soaks where you want histograms over time.

- [ ] **Step 1: Add Prometheus service**

`src/sidecar/prometheus.star`:

```python
def launch(plan, fuzz_service_name, scrape_interval_s = 5):
    cfg = """\
global:
  scrape_interval: {s}s
scrape_configs:
  - job_name: fuzz
    static_configs:
      - targets: ["{f}:8081"]
""".format(s = scrape_interval_s, f = fuzz_service_name)
    cfg_artifact = plan.render_templates(
        name = "prometheus-config",
        config = {"prometheus.yml": struct(template = cfg, data = {})},
    )
    return plan.add_service(
        name = "prometheus",
        config = ServiceConfig(
            image = "prom/prometheus:latest",
            ports = {"http": PortSpec(number = 9090, transport_protocol = "TCP", application_protocol = "http")},
            files = {"/etc/prometheus": cfg_artifact},
            cmd = ["--config.file=/etc/prometheus/prometheus.yml"],
        ),
    )
```

`src/sidecar/grafana.star` is analogous (image `grafana/grafana:latest`, port 3000, `GF_AUTH_ANONYMOUS_ENABLED=true`, `GF_AUTH_ANONYMOUS_ORG_ROLE=Viewer`). A pre-provisioned datasource pointing at `http://prometheus:9090` lives in `dashboard/grafana-provisioning/` (create as part of this task).

- [ ] **Step 2: Wire into `soak.star`**

```python
if args.get("enable_observability", False):
    prom = import_module("../sidecar/prometheus.star")
    graf = import_module("../sidecar/grafana.star")
    prom.launch(plan, "fuzz-soak")
    graf.launch(plan)
```

- [ ] **Step 3: Smoke-test**

```bash
make soak TX_RATE=3 ACCOUNTS=5 -- '{"soak_args":{"enable_observability":true,"corpus_host_path":"/tmp/fuzz-soak-corpus"}}'
# Open http://<grafana-ip>:3000, confirm Prometheus datasource is reachable
# and a query like rate(fuzz_txs_submitted_total[1m]) returns data.
make soak-down
```

(The `make soak` invocation above is a sketch; adjust the Makefile to forward extra suite args verbatim.)

- [ ] **Step 4: Commit**

```bash
git add src/sidecar/prometheus.star src/sidecar/grafana.star src/tests/soak.star dashboard/grafana-provisioning
git commit -m "soak: optional prometheus + grafana sidecars for long soak runs"
git push origin main
```

---

## Self-review checklist (executed before sign-off, not as a task)

1. **Spec coverage:**
   - Phase 1 (land merges): Tasks A1–A11 ✓
   - Phase 2 (crash detection): Tasks B1–B6 — classifier, runtime, runner hook, Kurtosis socket mount, hang-SIGQUIT ✓
   - Phase 3 (long-lived runner): Tasks C1–C5 — soak runner, rotation hook, MODE wiring, Kurtosis suite + host-bind, `make soak` ✓
   - Phase 4 (topology rebalance): Tasks D1–D2 — bumped to 10 keys, default 3 rippled + 2 goXRPL ✓
   - Phase 5 (Prometheus + dashboard panel): Tasks E1–E3 — `/metrics`, panel, optional Prometheus/Grafana ✓
2. **Placeholders:** `<NEW_SEED_*>`/`<NEW_PUBKEY_*>` are intentional in D1 — they are output of the keygen step run during the task itself, not a plan-failure placeholder. Every other step contains exact code or commands.
3. **Type/name consistency:** `runners.Config` extended with `CrashRuntime/CrashLabelKey/CrashLabelVal/CrashTailLines` (B4) and `Metrics` (E1); `runners.SoakConfig` embeds `Config` and adds `TxRate/RotateEvery` (C1). `corpus.Recorder.RecordDivergence(*Divergence)` matches existing M1 API. The crash event's `Kind` (`go_panic|rippled_fatal|sigsegv|sigabrt`) is set in B1 and read by metrics in E1's `Crashes` `impl` label. Prometheus collector names match the design doc verbatim.

---

## Out-of-scope follow-ups (referenced but not in this plan)

- M4 chaos runner (restart/netsplit/netem/amendment-flip).
- Multi-tier account pool (at-reserve / multisig / regular-key / blackholed) — Phase C's `RotateTiers` is a stub anchor for it.
- Mainnet-snapshot seeding (M5).
- Coverage-guided corpus mutations.
- Replacing `rpcclient.Submit*` helpers with xrpl-go-signed `tx_blob` submission (post-M1 follow-up #2).
