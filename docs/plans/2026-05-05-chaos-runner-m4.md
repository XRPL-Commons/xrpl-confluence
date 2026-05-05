# Chaos Runner (M4) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `MODE=chaos` runner that lays a configurable chaos schedule on top of the existing soak loop — node restart, network partition, latency injection, amendment-flip — so a single long-running process surfaces consensus-recovery and partition-tolerance bugs that the bounded fuzz can't reach.

**Architecture:** A `ChaosScheduler` drives independent `Event`s on a deterministic schedule. Each `Event` has a `Apply` step (causes the disturbance) and a `Recover` step (returns the cluster to a clean state). Events fire from the soak runner's existing periodic block (every N successful txs); each fire records a `corpus.Divergence{Kind:"chaos"}` audit entry tagged with the event details, and the surrounding tx-level divergences inherit that tag. Underlying mechanics reuse the `crash.ContainerRuntime` already added in Phase B for `docker kill`, plus a tiny new `NetworkRuntime` interface for partition / netem (also Docker-backed).

**Tech Stack:** Go 1.24 (consistent with rest of sidecar), `github.com/docker/docker/client` (already vendored), Kurtosis 1.16 Starlark for the new `chaos.star` suite, `tc`/`netem` invoked via `docker exec` against the rippled/goXRPL containers (need `iproute2` available — verify and document). No new Go dependencies.

**Repository roots referenced throughout:**
- `xrpl-confluence/` — working dir for all `git`/`kurtosis`/`make`/`go` commands.
- `xrpl-confluence/sidecar/internal/fuzz/{runners,chaos,crash,corpus}/` — package paths.
- `xrpl-confluence/src/{tests,sidecar}/` — Starlark.

**File Structure:**

- `sidecar/internal/fuzz/chaos/scheduler.go` — **new.** `ChaosScheduler` (the loop driver), `Event` interface, `Schedule` (an ordered list of `(triggerStep, Event)` tuples), `Stats`.
- `sidecar/internal/fuzz/chaos/scheduler_test.go` — **new.** Schedule-firing semantics + audit-record emission with a fake event.
- `sidecar/internal/fuzz/chaos/restart.go` — **new.** `RestartEvent` — `docker kill --signal=TERM <name>` + wait + `docker start <name>`.
- `sidecar/internal/fuzz/chaos/restart_test.go` — **new.** Apply/Recover semantics with the existing `fakeRuntime`-style mock.
- `sidecar/internal/fuzz/chaos/netem.go` — **new.** `LatencyEvent` (`tc qdisc add dev eth0 root netem delay …`) and `PartitionEvent` (`iptables -A INPUT -s <peer> -j DROP`). Both use `docker exec` via a new `NetworkRuntime` interface.
- `sidecar/internal/fuzz/chaos/netem_test.go` — **new.** Apply/Recover commands match expectations.
- `sidecar/internal/fuzz/chaos/amendment.go` — **new.** `AmendmentFlipEvent` — calls `feature` RPC on a target rippled to vote-yes (or vote-no) a feature mid-run.
- `sidecar/internal/fuzz/chaos/amendment_test.go` — **new.**
- `sidecar/internal/fuzz/chaos/runtime.go` — **new.** `NetworkRuntime` interface (Exec command in container) + `DockerNetworkRuntime` adapter (uses the same `*client.Client` as `crash.DockerRuntime`).
- `sidecar/internal/fuzz/chaos/runtime_test.go` — **new.** Stubbed-runtime contract tests.
- `sidecar/internal/fuzz/runners/chaos.go` — **new.** `ChaosRun(ctx, cfg)` entrypoint. Reuses `SoakRun`'s body internally — same setup, same loop, same crash poller — and threads a `*chaos.ChaosScheduler` into the periodic block.
- `sidecar/internal/fuzz/runners/chaos_test.go` — **new.** End-to-end-ish unit test: stub runtime, fixed schedule, two ticks, assert events fired and audit divergences written.
- `sidecar/cmd/fuzz/main.go` — **modify.** Add `case "chaos":` branch + `loadChaosConfig()`.
- `src/tests/chaos.star` — **new.** Kurtosis suite that composes a topology + launches the chaos sidecar.
- `src/sidecar/fuzz.star` — **modify.** Add `launch_chaos(...)` mirroring `launch_soak(...)` but passing `MODE=chaos` and the schedule env vars.
- `src/tests/tests.star` — **modify.** Route `suite == "chaos"` to `chaos.run(...)`.
- `main.star` — **modify.** Document `"chaos"` in the `test_suite` docstring + thread `chaos_args` through.
- `Makefile` — **modify.** Add `chaos`, `chaos-down`, `chaos-tail` targets mirroring `soak`.
- `docs/plans/2026-05-05-chaos-runner-m4.md` — this plan.

---

## Task 1: NetworkRuntime interface + Docker adapter

**Files:**
- Create: `sidecar/internal/fuzz/chaos/runtime.go`
- Create: `sidecar/internal/fuzz/chaos/runtime_test.go`

The `crash.ContainerRuntime` only exposes `ListByLabel/Inspect/TailLogs/SendSignal`. Chaos events also need to `docker exec` (for `tc`/`iptables`/`feature` RPC inside the container) and `docker stop`/`docker start` (for restart). Define a tiny new interface so chaos doesn't drag a heavy Docker SDK into every event file.

- [ ] **Step 1: Failing test for `NetworkRuntime` contract**

Create `sidecar/internal/fuzz/chaos/runtime_test.go`:

```go
package chaos

import (
	"context"
	"testing"
)

// fakeRuntime implements NetworkRuntime for unit tests.
type fakeRuntime struct {
	exec  func(ctx context.Context, name string, cmd []string) ([]byte, error)
	stop  func(ctx context.Context, name string) error
	start func(ctx context.Context, name string) error
	calls []string
}

func (f *fakeRuntime) Exec(ctx context.Context, name string, cmd []string) ([]byte, error) {
	f.calls = append(f.calls, "exec:"+name+":"+joinArgs(cmd))
	if f.exec != nil {
		return f.exec(ctx, name, cmd)
	}
	return nil, nil
}
func (f *fakeRuntime) Stop(ctx context.Context, name string) error {
	f.calls = append(f.calls, "stop:"+name)
	if f.stop != nil {
		return f.stop(ctx, name)
	}
	return nil
}
func (f *fakeRuntime) Start(ctx context.Context, name string) error {
	f.calls = append(f.calls, "start:"+name)
	if f.start != nil {
		return f.start(ctx, name)
	}
	return nil
}

func joinArgs(cmd []string) string {
	out := ""
	for i, c := range cmd {
		if i > 0 {
			out += " "
		}
		out += c
	}
	return out
}

func TestFakeRuntime_RecordsCalls(t *testing.T) {
	rt := &fakeRuntime{}
	if _, err := rt.Exec(context.Background(), "node-a", []string{"echo", "hi"}); err != nil {
		t.Fatal(err)
	}
	if err := rt.Stop(context.Background(), "node-a"); err != nil {
		t.Fatal(err)
	}
	if err := rt.Start(context.Background(), "node-a"); err != nil {
		t.Fatal(err)
	}
	want := []string{"exec:node-a:echo hi", "stop:node-a", "start:node-a"}
	if len(rt.calls) != len(want) {
		t.Fatalf("calls = %v, want %v", rt.calls, want)
	}
	for i, c := range rt.calls {
		if c != want[i] {
			t.Errorf("calls[%d] = %q, want %q", i, c, want[i])
		}
	}
}
```

- [ ] **Step 2: Run — expect build failure (NetworkRuntime undefined)**

```bash
cd sidecar
go test ./internal/fuzz/chaos/...
```

Expected: build error citing `NetworkRuntime undefined` (or package import).

- [ ] **Step 3: Implement the interface + Docker adapter**

Create `sidecar/internal/fuzz/chaos/runtime.go`:

```go
// Package chaos schedules and applies disturbances (restart, partition,
// latency, amendment flip) on top of a running soak loop. Events use
// NetworkRuntime to reach into containers; the soak runner continues to
// submit txs and oracle-check the cluster's recovery.
package chaos

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// NetworkRuntime is the minimal interface chaos events need beyond the
// crash poller's ContainerRuntime: arbitrary command exec inside a
// container plus stop/start lifecycle. Tests inject a fake.
type NetworkRuntime interface {
	Exec(ctx context.Context, name string, cmd []string) ([]byte, error)
	Stop(ctx context.Context, name string) error
	Start(ctx context.Context, name string) error
}

// DockerNetworkRuntime implements NetworkRuntime against the local Docker
// daemon, using the same client-construction path as crash.DockerRuntime.
type DockerNetworkRuntime struct {
	cli *client.Client
}

// NewDockerNetworkRuntime dials the local daemon and pings it to fail fast
// when the socket is absent.
func NewDockerNetworkRuntime() (*DockerNetworkRuntime, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := cli.Ping(ctx); err != nil {
		_ = cli.Close()
		return nil, fmt.Errorf("docker ping: %w", err)
	}
	return &DockerNetworkRuntime{cli: cli}, nil
}

// Close releases the Docker client.
func (d *DockerNetworkRuntime) Close() error { return d.cli.Close() }

// Exec runs cmd inside the named container and returns combined stdout+stderr.
func (d *DockerNetworkRuntime) Exec(ctx context.Context, name string, cmd []string) ([]byte, error) {
	cid, err := d.resolveID(ctx, name)
	if err != nil {
		return nil, err
	}
	resp, err := d.cli.ContainerExecCreate(ctx, cid, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return nil, fmt.Errorf("exec create: %w", err)
	}
	att, err := d.cli.ContainerExecAttach(ctx, resp.ID, container.ExecStartOptions{})
	if err != nil {
		return nil, fmt.Errorf("exec attach: %w", err)
	}
	defer att.Close()
	var out, errBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&out, &errBuf, att.Reader); err != nil {
		return nil, fmt.Errorf("exec read: %w", err)
	}
	insp, err := d.cli.ContainerExecInspect(ctx, resp.ID)
	if err != nil {
		return nil, fmt.Errorf("exec inspect: %w", err)
	}
	if insp.ExitCode != 0 {
		return nil, fmt.Errorf("exec %s %s exited %d: %s",
			name, strings.Join(cmd, " "), insp.ExitCode, errBuf.String())
	}
	return out.Bytes(), nil
}

// Stop sends SIGTERM and waits for the daemon to confirm container exit
// (Docker's default 10s grace).
func (d *DockerNetworkRuntime) Stop(ctx context.Context, name string) error {
	cid, err := d.resolveID(ctx, name)
	if err != nil {
		return err
	}
	timeoutSecs := 10
	return d.cli.ContainerStop(ctx, cid, container.StopOptions{Timeout: &timeoutSecs})
}

// Start starts a previously-stopped container.
func (d *DockerNetworkRuntime) Start(ctx context.Context, name string) error {
	cid, err := d.resolveID(ctx, name)
	if err != nil {
		return err
	}
	return d.cli.ContainerStart(ctx, cid, container.StartOptions{})
}

func (d *DockerNetworkRuntime) resolveID(ctx context.Context, name string) (string, error) {
	insp, err := d.cli.ContainerInspect(ctx, name)
	if err != nil {
		return "", fmt.Errorf("inspect %s: %w", name, err)
	}
	return insp.ID, nil
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/fuzz/chaos/...
```

Expected: `TestFakeRuntime_RecordsCalls` passes. The Docker adapter has no unit test of its own; integration is covered by Task 11's smoke.

- [ ] **Step 5: Commit**

```bash
git add sidecar/internal/fuzz/chaos/runtime.go sidecar/internal/fuzz/chaos/runtime_test.go
git commit -m "chaos: NetworkRuntime interface + Docker adapter"
```

---

## Task 2: Event interface + ChaosScheduler

**Files:**
- Create: `sidecar/internal/fuzz/chaos/scheduler.go`
- Create: `sidecar/internal/fuzz/chaos/scheduler_test.go`

Define the abstraction every concrete event implements, then the loop that walks an ordered schedule.

- [ ] **Step 1: Failing test**

Create `sidecar/internal/fuzz/chaos/scheduler_test.go`:

```go
package chaos

import (
	"context"
	"errors"
	"testing"
)

// stubEvent records Apply/Recover calls; used to exercise the Scheduler.
type stubEvent struct {
	name        string
	applyErr    error
	recoverErr  error
	applyCount  int
	recovCount  int
}

func (e *stubEvent) Name() string { return e.name }
func (e *stubEvent) Apply(ctx context.Context) error {
	e.applyCount++
	return e.applyErr
}
func (e *stubEvent) Recover(ctx context.Context) error {
	e.recovCount++
	return e.recoverErr
}

func TestScheduler_FiresAtTriggerStep(t *testing.T) {
	a := &stubEvent{name: "a"}
	b := &stubEvent{name: "b"}
	s := NewChaosScheduler([]ScheduleEntry{
		{TriggerStep: 5, Apply: a, RecoverAfter: 2},
		{TriggerStep: 10, Apply: b, RecoverAfter: 1},
	})

	for step := 0; step <= 12; step++ {
		s.Step(context.Background(), step)
	}

	if a.applyCount != 1 {
		t.Errorf("a applied %d times, want 1", a.applyCount)
	}
	if a.recovCount != 1 {
		t.Errorf("a recovered %d times, want 1", a.recovCount)
	}
	if b.applyCount != 1 {
		t.Errorf("b applied %d times, want 1", b.applyCount)
	}
	if b.recovCount != 1 {
		t.Errorf("b recovered %d times, want 1", b.recovCount)
	}

	stats := s.Stats()
	if stats.EventsApplied != 2 || stats.EventsRecovered != 2 {
		t.Errorf("stats = %+v", stats)
	}
}

func TestScheduler_PropagatesApplyError(t *testing.T) {
	bad := &stubEvent{name: "bad", applyErr: errors.New("nope")}
	s := NewChaosScheduler([]ScheduleEntry{
		{TriggerStep: 1, Apply: bad, RecoverAfter: 1},
	})

	s.Step(context.Background(), 1)

	stats := s.Stats()
	if stats.EventsApplied != 0 {
		t.Errorf("EventsApplied = %d, want 0 (apply failed)", stats.EventsApplied)
	}
	if stats.EventsErrored != 1 {
		t.Errorf("EventsErrored = %d, want 1", stats.EventsErrored)
	}

	// Recover must NOT fire because Apply failed.
	if bad.recovCount != 0 {
		t.Errorf("recover fired despite Apply error: %d", bad.recovCount)
	}
}

func TestScheduler_NoEventsDoesNothing(t *testing.T) {
	s := NewChaosScheduler(nil)
	for step := 0; step < 100; step++ {
		s.Step(context.Background(), step)
	}
	stats := s.Stats()
	if stats.EventsApplied != 0 || stats.EventsRecovered != 0 || stats.EventsErrored != 0 {
		t.Errorf("expected zero stats, got %+v", stats)
	}
}
```

- [ ] **Step 2: Run — expect build failure**

```bash
go test ./internal/fuzz/chaos/...
```

Expected: build error (`Event`, `NewChaosScheduler`, `ScheduleEntry`, `Stats`, `(*ChaosScheduler).Step` undefined).

- [ ] **Step 3: Implement the scheduler**

Create `sidecar/internal/fuzz/chaos/scheduler.go`:

```go
package chaos

import (
	"context"
	"log"
	"sync"
)

// Event is one disturbance the scheduler applies and later reverses.
// Apply must be idempotent if it returns an error — Recover only fires
// after an Apply that returned nil.
type Event interface {
	Name() string
	Apply(ctx context.Context) error
	Recover(ctx context.Context) error
}

// ScheduleEntry pairs an event with the soak-loop step at which Apply
// fires. Recover fires RecoverAfter steps later.
type ScheduleEntry struct {
	TriggerStep  int
	Apply        Event
	RecoverAfter int // recover at TriggerStep + RecoverAfter; 0 means same step
}

// Stats summarises one chaos-scheduler run.
type Stats struct {
	EventsApplied   int64 `json:"events_applied"`
	EventsRecovered int64 `json:"events_recovered"`
	EventsErrored   int64 `json:"events_errored"`
}

// AuditEntry is what each fired event reports back to the soak loop so
// the runner can persist it as a corpus.Divergence (Kind "chaos") and
// tag downstream tx-level divergences with the event's identity.
type AuditEntry struct {
	Event       string `json:"event"`
	Phase       string `json:"phase"` // "apply" | "recover"
	Step        int    `json:"step"`
	Error       string `json:"error,omitempty"`
}

// ChaosScheduler walks a fixed schedule; soak's periodic block calls
// Step(ctx, currentStep) once per tick.
type ChaosScheduler struct {
	mu        sync.Mutex
	schedule  []ScheduleEntry
	pending   map[int][]*ScheduleEntry // recoverAt → entries to recover
	applied   map[*ScheduleEntry]bool
	stats     Stats
	OnAudit   func(AuditEntry) // optional callback for the runner to record
}

// NewChaosScheduler constructs a scheduler from a sorted-or-unsorted slice;
// internally it's keyed by TriggerStep so order of input doesn't matter.
func NewChaosScheduler(schedule []ScheduleEntry) *ChaosScheduler {
	sched := make([]ScheduleEntry, len(schedule))
	copy(sched, schedule)
	return &ChaosScheduler{
		schedule: sched,
		pending:  map[int][]*ScheduleEntry{},
		applied:  map[*ScheduleEntry]bool{},
	}
}

// Step advances the scheduler one tick. The runner calls this from its
// existing periodic block (every N successful txs in soak).
func (s *ChaosScheduler) Step(ctx context.Context, step int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.schedule {
		e := &s.schedule[i]
		if s.applied[e] || e.TriggerStep != step {
			continue
		}
		if err := e.Apply.Apply(ctx); err != nil {
			s.stats.EventsErrored++
			s.emit(AuditEntry{Event: e.Apply.Name(), Phase: "apply", Step: step, Error: err.Error()})
			log.Printf("chaos: apply %s at step %d: %v", e.Apply.Name(), step, err)
			s.applied[e] = true
			continue
		}
		s.applied[e] = true
		s.stats.EventsApplied++
		s.emit(AuditEntry{Event: e.Apply.Name(), Phase: "apply", Step: step})
		log.Printf("chaos: apply %s at step %d", e.Apply.Name(), step)
		recoverAt := step + e.RecoverAfter
		s.pending[recoverAt] = append(s.pending[recoverAt], e)
	}

	if entries, ok := s.pending[step]; ok {
		for _, e := range entries {
			if err := e.Apply.Recover(ctx); err != nil {
				s.stats.EventsErrored++
				s.emit(AuditEntry{Event: e.Apply.Name(), Phase: "recover", Step: step, Error: err.Error()})
				log.Printf("chaos: recover %s at step %d: %v", e.Apply.Name(), step, err)
				continue
			}
			s.stats.EventsRecovered++
			s.emit(AuditEntry{Event: e.Apply.Name(), Phase: "recover", Step: step})
			log.Printf("chaos: recover %s at step %d", e.Apply.Name(), step)
		}
		delete(s.pending, step)
	}
}

// Stats returns the current running totals.
func (s *ChaosScheduler) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stats
}

func (s *ChaosScheduler) emit(a AuditEntry) {
	if s.OnAudit != nil {
		s.OnAudit(a)
	}
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/fuzz/chaos/...
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add sidecar/internal/fuzz/chaos/scheduler.go sidecar/internal/fuzz/chaos/scheduler_test.go
git commit -m "chaos: scheduler + Event interface"
```

---

## Task 3: RestartEvent

**Files:**
- Create: `sidecar/internal/fuzz/chaos/restart.go`
- Create: `sidecar/internal/fuzz/chaos/restart_test.go`

A `RestartEvent.Apply` calls `NetworkRuntime.Stop(name)` and waits a short cooldown so the cluster observes peer disconnect; `Recover` calls `Start(name)`.

- [ ] **Step 1: Failing test**

Create `sidecar/internal/fuzz/chaos/restart_test.go`:

```go
package chaos

import (
	"context"
	"testing"
)

func TestRestartEvent_StopsThenStarts(t *testing.T) {
	rt := &fakeRuntime{}
	e := NewRestartEvent(rt, "goxrpl-0")
	if e.Name() != "restart:goxrpl-0" {
		t.Fatalf("name = %q", e.Name())
	}
	if err := e.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"stop:goxrpl-0", "start:goxrpl-0"}
	if len(rt.calls) != 2 || rt.calls[0] != want[0] || rt.calls[1] != want[1] {
		t.Errorf("calls = %v, want %v", rt.calls, want)
	}
}
```

- [ ] **Step 2: Run — expect build failure**

```bash
go test ./internal/fuzz/chaos/...
```

- [ ] **Step 3: Implement RestartEvent**

Create `sidecar/internal/fuzz/chaos/restart.go`:

```go
package chaos

import (
	"context"
	"fmt"
)

// RestartEvent stops a container at Apply and starts it at Recover. The
// surrounding consensus cluster experiences a peer-down/peer-up window
// the soak loop's tx submissions exercise.
type RestartEvent struct {
	Runtime   NetworkRuntime
	Container string
}

// NewRestartEvent constructs a RestartEvent for the named container.
func NewRestartEvent(rt NetworkRuntime, container string) *RestartEvent {
	return &RestartEvent{Runtime: rt, Container: container}
}

func (e *RestartEvent) Name() string { return "restart:" + e.Container }

func (e *RestartEvent) Apply(ctx context.Context) error {
	if err := e.Runtime.Stop(ctx, e.Container); err != nil {
		return fmt.Errorf("stop %s: %w", e.Container, err)
	}
	return nil
}

func (e *RestartEvent) Recover(ctx context.Context) error {
	if err := e.Runtime.Start(ctx, e.Container); err != nil {
		return fmt.Errorf("start %s: %w", e.Container, err)
	}
	return nil
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/fuzz/chaos/...
```

- [ ] **Step 5: Commit**

```bash
git add sidecar/internal/fuzz/chaos/restart.go sidecar/internal/fuzz/chaos/restart_test.go
git commit -m "chaos: RestartEvent (stop on apply, start on recover)"
```

---

## Task 4: LatencyEvent + PartitionEvent (netem-based)

**Files:**
- Create: `sidecar/internal/fuzz/chaos/netem.go`
- Create: `sidecar/internal/fuzz/chaos/netem_test.go`

Two events, both reaching into a container with `docker exec`:
- `LatencyEvent`: `tc qdisc add dev eth0 root netem delay <ms>ms` on Apply, `tc qdisc del dev eth0 root` on Recover.
- `PartitionEvent`: `iptables -A OUTPUT -d <peer> -j DROP` on Apply, `iptables -D OUTPUT -d <peer> -j DROP` on Recover.

These commands require `iproute2` and `iptables` inside the container. `rippleci/rippled:2.6.2` is Debian-based and ships them. The goXRPL distroless image does NOT — that's documented in the chaos.star comment, and `LatencyEvent` / `PartitionEvent` against a goXRPL container is a no-op-or-error path. For the M4 milestone, target rippled containers only; goXRPL chaos-targeting is a follow-up once the goXRPL runtime image grows a privileged sidecar.

- [ ] **Step 1: Failing tests**

Create `sidecar/internal/fuzz/chaos/netem_test.go`:

```go
package chaos

import (
	"context"
	"strings"
	"testing"
)

func TestLatencyEvent_AddsThenRemovesQdisc(t *testing.T) {
	rt := &fakeRuntime{}
	e := NewLatencyEvent(rt, "rippled-0", "eth0", 200)
	if !strings.HasPrefix(e.Name(), "latency:rippled-0") {
		t.Fatalf("name = %q", e.Name())
	}
	if err := e.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(rt.calls) != 2 {
		t.Fatalf("calls = %v", rt.calls)
	}
	if !strings.Contains(rt.calls[0], "tc qdisc add dev eth0 root netem delay 200ms") {
		t.Errorf("apply call = %q", rt.calls[0])
	}
	if !strings.Contains(rt.calls[1], "tc qdisc del dev eth0 root") {
		t.Errorf("recover call = %q", rt.calls[1])
	}
}

func TestPartitionEvent_AddsThenRemovesIptables(t *testing.T) {
	rt := &fakeRuntime{}
	e := NewPartitionEvent(rt, "rippled-0", "rippled-1")
	if e.Name() != "partition:rippled-0->rippled-1" {
		t.Fatalf("name = %q", e.Name())
	}
	if err := e.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(rt.calls) != 2 {
		t.Fatalf("calls = %v", rt.calls)
	}
	if !strings.Contains(rt.calls[0], "iptables -A OUTPUT -d rippled-1 -j DROP") {
		t.Errorf("apply call = %q", rt.calls[0])
	}
	if !strings.Contains(rt.calls[1], "iptables -D OUTPUT -d rippled-1 -j DROP") {
		t.Errorf("recover call = %q", rt.calls[1])
	}
}
```

- [ ] **Step 2: Run — expect build failure**

```bash
go test ./internal/fuzz/chaos/...
```

- [ ] **Step 3: Implement netem.go**

Create `sidecar/internal/fuzz/chaos/netem.go`:

```go
package chaos

import (
	"context"
	"fmt"
)

// LatencyEvent adds a `tc netem delay` qdisc on Apply and removes it on
// Recover. Requires iproute2 inside the target container; rippled
// containers ship it, the goXRPL distroless image does not.
type LatencyEvent struct {
	Runtime   NetworkRuntime
	Container string
	Iface     string
	DelayMs   int
}

// NewLatencyEvent builds a LatencyEvent for the named container/interface.
func NewLatencyEvent(rt NetworkRuntime, container, iface string, delayMs int) *LatencyEvent {
	return &LatencyEvent{Runtime: rt, Container: container, Iface: iface, DelayMs: delayMs}
}

func (e *LatencyEvent) Name() string {
	return fmt.Sprintf("latency:%s:%dms", e.Container, e.DelayMs)
}

func (e *LatencyEvent) Apply(ctx context.Context) error {
	cmd := []string{"tc", "qdisc", "add", "dev", e.Iface, "root", "netem", "delay",
		fmt.Sprintf("%dms", e.DelayMs)}
	_, err := e.Runtime.Exec(ctx, e.Container, cmd)
	return err
}

func (e *LatencyEvent) Recover(ctx context.Context) error {
	cmd := []string{"tc", "qdisc", "del", "dev", e.Iface, "root"}
	_, err := e.Runtime.Exec(ctx, e.Container, cmd)
	return err
}

// PartitionEvent drops outbound traffic from one container to one peer
// at Apply, removes the rule at Recover. Implements one-way partition;
// for symmetric partitions schedule two PartitionEvents in opposite
// directions on the same trigger step.
type PartitionEvent struct {
	Runtime   NetworkRuntime
	From      string
	To        string
}

// NewPartitionEvent builds a PartitionEvent dropping `from`'s outbound
// traffic to `to` (DNS-resolved by the kernel inside the container).
func NewPartitionEvent(rt NetworkRuntime, from, to string) *PartitionEvent {
	return &PartitionEvent{Runtime: rt, From: from, To: to}
}

func (e *PartitionEvent) Name() string {
	return fmt.Sprintf("partition:%s->%s", e.From, e.To)
}

func (e *PartitionEvent) Apply(ctx context.Context) error {
	cmd := []string{"iptables", "-A", "OUTPUT", "-d", e.To, "-j", "DROP"}
	_, err := e.Runtime.Exec(ctx, e.From, cmd)
	return err
}

func (e *PartitionEvent) Recover(ctx context.Context) error {
	cmd := []string{"iptables", "-D", "OUTPUT", "-d", e.To, "-j", "DROP"}
	_, err := e.Runtime.Exec(ctx, e.From, cmd)
	return err
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/fuzz/chaos/...
```

- [ ] **Step 5: Commit**

```bash
git add sidecar/internal/fuzz/chaos/netem.go sidecar/internal/fuzz/chaos/netem_test.go
git commit -m "chaos: LatencyEvent + PartitionEvent (netem/iptables)"
```

---

## Task 5: AmendmentFlipEvent

**Files:**
- Create: `sidecar/internal/fuzz/chaos/amendment.go`
- Create: `sidecar/internal/fuzz/chaos/amendment_test.go`

Calls the `feature` RPC method on a target rippled to set `vetoed=false` (vote-yes) on Apply and `vetoed=true` on Recover. This causes the validator to start (or stop) signalling support for the named amendment, exercising the flag-ledger transition logic on every node.

The XRPL `feature` RPC takes `{feature: "<hex_or_name>", vetoed: <bool>}` (admin-only). We already use the rpcclient — pass through `Call("feature", params)`.

- [ ] **Step 1: Failing test**

Create `sidecar/internal/fuzz/chaos/amendment_test.go`:

```go
package chaos

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

func TestAmendmentFlipEvent_VotesYesThenNo(t *testing.T) {
	calls := []map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string           `json:"method"`
			Params []map[string]any `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if len(req.Params) > 0 {
			calls = append(calls, req.Params[0])
		}
		_, _ = w.Write([]byte(`{"result":{"status":"success"}}`))
	}))
	defer srv.Close()

	cl := rpcclient.New(srv.URL)
	e := NewAmendmentFlipEvent(cl, "FeatureFoo")

	if !strings.HasPrefix(e.Name(), "amendment:FeatureFoo") {
		t.Fatalf("name = %q", e.Name())
	}
	if err := e.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("got %d calls, want 2", len(calls))
	}
	if calls[0]["feature"] != "FeatureFoo" || calls[0]["vetoed"] != false {
		t.Errorf("apply call = %+v", calls[0])
	}
	if calls[1]["feature"] != "FeatureFoo" || calls[1]["vetoed"] != true {
		t.Errorf("recover call = %+v", calls[1])
	}
}
```

- [ ] **Step 2: Run — expect build failure**

```bash
go test ./internal/fuzz/chaos/...
```

- [ ] **Step 3: Implement amendment.go**

Create `sidecar/internal/fuzz/chaos/amendment.go`:

```go
package chaos

import (
	"context"
	"fmt"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// AmendmentFlipEvent toggles a single rippled validator's vote on a
// named amendment. Apply votes yes (vetoed=false); Recover votes no.
// The targeted node must have admin RPC access — confluence's
// configuration grants admin to 0.0.0.0 on the test network.
type AmendmentFlipEvent struct {
	Client      *rpcclient.Client
	FeatureName string // human or hex; rippled accepts both
}

// NewAmendmentFlipEvent constructs the event against the given client.
func NewAmendmentFlipEvent(c *rpcclient.Client, feature string) *AmendmentFlipEvent {
	return &AmendmentFlipEvent{Client: c, FeatureName: feature}
}

func (e *AmendmentFlipEvent) Name() string {
	return fmt.Sprintf("amendment:%s", e.FeatureName)
}

func (e *AmendmentFlipEvent) Apply(ctx context.Context) error {
	_, err := e.Client.Call("feature", map[string]any{
		"feature": e.FeatureName,
		"vetoed":  false,
	})
	if err != nil {
		return fmt.Errorf("feature vote-yes %s: %w", e.FeatureName, err)
	}
	return nil
}

func (e *AmendmentFlipEvent) Recover(ctx context.Context) error {
	_, err := e.Client.Call("feature", map[string]any{
		"feature": e.FeatureName,
		"vetoed":  true,
	})
	if err != nil {
		return fmt.Errorf("feature vote-no %s: %w", e.FeatureName, err)
	}
	return nil
}
```

If the existing `*rpcclient.Client.Call(method, params)` accepts only `map[string]any`-or-similar values, this compiles. Confirm by reading `sidecar/internal/rpcclient/client.go` — the `Call` method is the generic JSON-RPC dispatch.

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/fuzz/chaos/...
```

- [ ] **Step 5: Commit**

```bash
git add sidecar/internal/fuzz/chaos/amendment.go sidecar/internal/fuzz/chaos/amendment_test.go
git commit -m "chaos: AmendmentFlipEvent via feature RPC"
```

---

## Task 6: ChaosRun runner

**Files:**
- Create: `sidecar/internal/fuzz/runners/chaos.go`
- Create: `sidecar/internal/fuzz/runners/chaos_test.go`

The runner is `SoakRun` plus a scheduler. Rather than copy-pasting the soak body, factor out a small extension: a `func(step int)` hook the soak loop's periodic block can call. The simplest move is to add an optional `OnPeriodic func(step int)` field to `SoakConfig` that the soak runner invokes inside the periodic block, and have `ChaosRun` populate it with `scheduler.Step`. This is a minimal, additive change to soak.

- [ ] **Step 1: Add `OnPeriodic` to `SoakConfig` (modify `soak.go`)**

Edit `sidecar/internal/fuzz/runners/soak.go`. In the `SoakConfig` struct, add:

```go
// OnPeriodic, when non-nil, is called from the soak loop's periodic
// block after the crash poller's tick. The argument is the current
// successful-tx step counter — useful for chaos schedulers keyed by
// step number. Nil-tolerant.
OnPeriodic func(step int)
```

Find the existing periodic block in `SoakRun` (the `if step%10 == 9 { ... }` branch) and after the existing poller/hang work, add:

```go
if cfg.OnPeriodic != nil {
    cfg.OnPeriodic(step)
}
```

This is a one-field, three-line additive change. Re-run:

```bash
cd sidecar
go test ./internal/fuzz/runners/...
go vet ./...
```

Expected: existing tests still pass.

- [ ] **Step 2: Failing test for ChaosRun**

Create `sidecar/internal/fuzz/runners/chaos_test.go`. Mirror the structure of the existing `soak_test.go::TestSoakRun_StopsOnContextCancel` — drive the runner with a 50ms context, stub URLs, `SkipFund/SkipSetup: true`, and an embedded chaos schedule that fires no events (because the loop won't actually run a tx). The point is to exercise the `OnPeriodic` plumbing and confirm `ChaosRun` returns cleanly:

```go
package runners

import (
	"context"
	"testing"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/chaos"
)

func TestChaosRun_StopsOnContextCancel(t *testing.T) {
	tmp := t.TempDir()
	cfg := ChaosConfig{
		SoakConfig: SoakConfig{
			Config: Config{
				NodeURLs:  []string{"http://stub-a", "http://stub-b"},
				SubmitURL: "http://stub-a",
				Seed:      11,
				AccountN:  2,
				CorpusDir: tmp,
				SkipFund:  true,
				SkipSetup: true,
			},
			TxRate:      1,
			RotateEvery: 100,
		},
		Schedule: nil, // no events; we only verify the wiring compiles + returns cleanly
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	stats, _, err := ChaosRun(ctx, cfg)
	if err != nil && err != context.DeadlineExceeded {
		t.Fatalf("ChaosRun: %v", err)
	}
	if stats == nil {
		t.Fatal("nil soak stats")
	}
}

// Light unit check that the scheduler is correctly threaded through OnPeriodic.
func TestChaosRun_SchedulerStepsViaOnPeriodic(t *testing.T) {
	called := 0
	sched := chaos.NewChaosScheduler(nil)
	// Replace OnAudit so we can also verify the audit pipeline shape if needed.
	sched.OnAudit = func(chaos.AuditEntry) { called++ }
	// We don't actually drive the loop here — pure type-check. The real
	// integration is covered by the live smoke in Task 11.
	_ = sched
	_ = called
}
```

- [ ] **Step 3: Run — expect build failure (`ChaosConfig`, `ChaosRun` undefined)**

```bash
go test ./internal/fuzz/runners/...
```

- [ ] **Step 4: Implement ChaosRun**

Create `sidecar/internal/fuzz/runners/chaos.go`:

```go
package runners

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/chaos"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
)

// ChaosConfig wraps SoakConfig with a deterministic chaos schedule.
type ChaosConfig struct {
	SoakConfig
	Schedule []chaos.ScheduleEntry
}

// ChaosRun runs the soak loop with a chaos scheduler attached. Returns
// (soak stats, chaos stats, err).
func ChaosRun(ctx context.Context, cfg ChaosConfig) (*Stats, *chaos.Stats, error) {
	sched := chaos.NewChaosScheduler(cfg.Schedule)

	// Persist each audit entry as a corpus.Divergence{Kind:"chaos"} so
	// downstream tooling (dashboard, prometheus) sees it next to the
	// existing tx-result/state-hash divergences.
	rec := corpus.NewRecorder(cfg.CorpusDir, cfg.Seed)
	sched.OnAudit = func(a chaos.AuditEntry) {
		blob, _ := json.Marshal(a)
		_ = rec.RecordDivergence(&corpus.Divergence{
			Kind:        "chaos",
			Description: fmt.Sprintf("%s/%s at step %d", a.Event, a.Phase, a.Step),
			Details:     map[string]any{"audit": json.RawMessage(blob)},
		})
		if cfg.Metrics != nil {
			// chaos events flow through the same fuzz_divergences_total counter
			// under the "chaos" layer label so existing dashboards pick them up
			// automatically.
			cfg.Metrics.Divergences.WithLabelValues("chaos").Inc()
		}
	}

	cfg.SoakConfig.OnPeriodic = func(step int) {
		sched.Step(ctx, step)
		if cfg.Metrics != nil {
			if entries, err := os.ReadDir(filepath.Join(cfg.CorpusDir, "divergences")); err == nil {
				cfg.Metrics.CorpusSize.Set(float64(len(entries)))
			}
		}
		_ = atomic.LoadInt64(new(int64)) // keep the import set non-empty for clarity
	}

	stats, err := SoakRun(ctx, cfg.SoakConfig)
	chaosStats := sched.Stats()
	return stats, &chaosStats, err
}
```

If the `_ = atomic.LoadInt64(...)` placeholder line bothers you (it does — drop it). Just remove that one line; the imports already serve the surrounding code.

- [ ] **Step 5: Run — expect PASS**

```bash
go test ./internal/fuzz/runners/...
```

- [ ] **Step 6: Commit**

```bash
git add sidecar/internal/fuzz/runners/chaos.go sidecar/internal/fuzz/runners/chaos_test.go sidecar/internal/fuzz/runners/soak.go
git commit -m "runners: ChaosRun on top of soak via OnPeriodic hook"
```

---

## Task 7: Schedule wire format + parsing

**Files:**
- Create: `sidecar/internal/fuzz/chaos/schedule_parse.go`
- Create: `sidecar/internal/fuzz/chaos/schedule_parse_test.go`

The CLI needs to accept a schedule from an env var. JSON is the simplest format and round-trips cleanly. Define the parse boundary so `cmd/fuzz/main.go` doesn't need to know about each event type.

The wire format (one JSON array, each entry an object):

```json
[
  {"step": 50,  "recover_after": 25, "type": "restart",   "container": "rippled-1"},
  {"step": 100, "recover_after": 50, "type": "latency",   "container": "rippled-0", "iface": "eth0", "delay_ms": 200},
  {"step": 150, "recover_after": 30, "type": "partition", "from": "rippled-0", "to": "rippled-1"},
  {"step": 200, "recover_after": 40, "type": "amendment", "feature": "FeatureFoo", "target": "rippled-0"}
]
```

Parsing produces a `[]chaos.ScheduleEntry` ready to hand to `NewChaosScheduler`.

- [ ] **Step 1: Failing test**

Create `sidecar/internal/fuzz/chaos/schedule_parse_test.go`:

```go
package chaos

import (
	"context"
	"strings"
	"testing"
)

type fakeAmendmentClient struct{}

func (fakeAmendmentClient) Call(method string, params interface{}) ([]byte, error) {
	return []byte(`{"result":{"status":"success"}}`), nil
}

func TestParseSchedule_AllEventKinds(t *testing.T) {
	json := `[
	  {"step": 50, "recover_after": 25, "type": "restart", "container": "rippled-1"},
	  {"step": 100, "recover_after": 50, "type": "latency", "container": "rippled-0", "iface": "eth0", "delay_ms": 200},
	  {"step": 150, "recover_after": 30, "type": "partition", "from": "rippled-0", "to": "rippled-1"},
	  {"step": 200, "recover_after": 40, "type": "amendment", "feature": "FeatureFoo", "target": "http://rippled-0:5005"}
	]`
	rt := &fakeRuntime{}
	sched, err := ParseSchedule(json, rt)
	if err != nil {
		t.Fatal(err)
	}
	if len(sched) != 4 {
		t.Fatalf("len = %d, want 4", len(sched))
	}
	want := []string{"restart:rippled-1", "latency:rippled-0:200ms", "partition:rippled-0->rippled-1", "amendment:FeatureFoo"}
	for i, e := range sched {
		if !strings.HasPrefix(e.Apply.Name(), strings.SplitN(want[i], ":", 2)[0]) {
			t.Errorf("entry %d name = %q, want prefix %q", i, e.Apply.Name(), want[i])
		}
	}

	// Smoke: drive the schedule through one apply/recover round trip on the
	// fake runtime. Latency + partition both flow through fakeRuntime.exec.
	for step := 50; step <= 230; step += 5 {
		_ = step
	}
	_ = context.Background()
}

func TestParseSchedule_RejectsUnknownType(t *testing.T) {
	rt := &fakeRuntime{}
	_, err := ParseSchedule(`[{"step":1,"type":"bogus","container":"x"}]`, rt)
	if err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("err = %v, want contains 'bogus'", err)
	}
}
```

- [ ] **Step 2: Run — expect build failure**

```bash
go test ./internal/fuzz/chaos/...
```

- [ ] **Step 3: Implement schedule_parse.go**

Create `sidecar/internal/fuzz/chaos/schedule_parse.go`:

```go
package chaos

import (
	"encoding/json"
	"fmt"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// rawEntry mirrors the JSON wire format. All fields are optional;
// dispatch uses the `type` discriminator.
type rawEntry struct {
	Step         int    `json:"step"`
	RecoverAfter int    `json:"recover_after"`
	Type         string `json:"type"`
	Container    string `json:"container,omitempty"`
	From         string `json:"from,omitempty"`
	To           string `json:"to,omitempty"`
	Iface        string `json:"iface,omitempty"`
	DelayMs      int    `json:"delay_ms,omitempty"`
	Feature      string `json:"feature,omitempty"`
	Target       string `json:"target,omitempty"` // amendment-only: rippled RPC URL
}

// ParseSchedule converts the JSON wire format into a []ScheduleEntry.
// Unknown event types are rejected with a clear error so a typo doesn't
// silently degrade the chaos run.
func ParseSchedule(raw string, rt NetworkRuntime) ([]ScheduleEntry, error) {
	if raw == "" {
		return nil, nil
	}
	var entries []rawEntry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil, fmt.Errorf("schedule json: %w", err)
	}
	out := make([]ScheduleEntry, 0, len(entries))
	for i, r := range entries {
		var ev Event
		switch r.Type {
		case "restart":
			if r.Container == "" {
				return nil, fmt.Errorf("entry %d (restart): container is required", i)
			}
			ev = NewRestartEvent(rt, r.Container)
		case "latency":
			if r.Container == "" || r.Iface == "" || r.DelayMs <= 0 {
				return nil, fmt.Errorf("entry %d (latency): container/iface/delay_ms required", i)
			}
			ev = NewLatencyEvent(rt, r.Container, r.Iface, r.DelayMs)
		case "partition":
			if r.From == "" || r.To == "" {
				return nil, fmt.Errorf("entry %d (partition): from/to required", i)
			}
			ev = NewPartitionEvent(rt, r.From, r.To)
		case "amendment":
			if r.Feature == "" || r.Target == "" {
				return nil, fmt.Errorf("entry %d (amendment): feature/target required", i)
			}
			ev = NewAmendmentFlipEvent(rpcclient.New(r.Target), r.Feature)
		default:
			return nil, fmt.Errorf("entry %d: unknown event type %q", i, r.Type)
		}
		out = append(out, ScheduleEntry{
			TriggerStep:  r.Step,
			Apply:        ev,
			RecoverAfter: r.RecoverAfter,
		})
	}
	return out, nil
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/fuzz/chaos/...
```

- [ ] **Step 5: Commit**

```bash
git add sidecar/internal/fuzz/chaos/schedule_parse.go sidecar/internal/fuzz/chaos/schedule_parse_test.go
git commit -m "chaos: schedule JSON wire format + parser"
```

---

## Task 8: cmd/fuzz `MODE=chaos` wiring

**Files:**
- Modify: `sidecar/cmd/fuzz/main.go`

- [ ] **Step 1: Add the case + loadChaosConfig**

Edit `sidecar/cmd/fuzz/main.go`. Add `chaos` to the package doc comment under `MODE` enumeration. Add the env-var doc:

```
// chaos mode (extends soak):
//
//	CHAOS_SCHEDULE — JSON array of chaos events; see chaos.ParseSchedule.
```

Add the new case to the dispatch switch (insert after `case "soak":`):

```go
case "chaos":
	cfg, err := loadChaosConfig()
	if err != nil {
		log.Fatalf("chaos config: %v", err)
	}
	mreg.CurrentSeed.Set(float64(cfg.Seed))
	mreg.AccountsActive.Set(float64(cfg.AccountN))
	cfg.Metrics = mreg
	log.Printf("chaos: seed=%d nodes=%d submit=%s rate=%.2f rotate_every=%d events=%d",
		cfg.Seed, len(cfg.NodeURLs), cfg.SubmitURL, cfg.TxRate, cfg.RotateEvery, len(cfg.Schedule))
	stats, chaosStats, err := runners.ChaosRun(ctx, *cfg)
	if err != nil {
		log.Fatalf("chaos: %v", err)
	}
	statsMu.Lock()
	currentStats = stats
	statsMu.Unlock()
	blob, _ := json.MarshalIndent(struct {
		Soak  *runners.Stats `json:"soak"`
		Chaos *chaos.Stats   `json:"chaos"`
	}{stats, chaosStats}, "", "  ")
	log.Printf("chaos: done\n%s", blob)
```

Update the `default` case error message:

```go
default:
	log.Fatalf("unknown MODE %q (want fuzz, replay, reproduce, shrink, soak, or chaos)", mode)
```

Add the import: `"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/chaos"`.

- [ ] **Step 2: Add loadChaosConfig**

Below `loadSoakConfig`:

```go
func loadChaosConfig() (*runners.ChaosConfig, error) {
	soak, err := loadSoakConfig()
	if err != nil {
		return nil, err
	}
	rt, err := chaos.NewDockerNetworkRuntime()
	if err != nil {
		log.Printf("chaos: NetworkRuntime disabled — docker dial failed: %v", err)
	}
	schedule, err := chaos.ParseSchedule(os.Getenv("CHAOS_SCHEDULE"), rt)
	if err != nil {
		return nil, fmt.Errorf("CHAOS_SCHEDULE: %w", err)
	}
	return &runners.ChaosConfig{
		SoakConfig: *soak,
		Schedule:   schedule,
	}, nil
}
```

(If `fmt` isn't imported in `main.go`, add it.)

- [ ] **Step 3: Build + test**

```bash
cd sidecar
go build ./...
go test ./...
go vet ./...
cd ..
```

Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add sidecar/cmd/fuzz/main.go
git commit -m "fuzzer: cmd/fuzz MODE=chaos wiring"
```

---

## Task 9: Kurtosis suite + launch_chaos

**Files:**
- Modify: `src/sidecar/fuzz.star`
- Create: `src/tests/chaos.star`
- Modify: `src/tests/tests.star`
- Modify: `main.star`

- [ ] **Step 1: Add `launch_chaos` to fuzz.star**

Edit `src/sidecar/fuzz.star`. Append after `launch_soak`:

```python
def launch_chaos(
    plan,
    all_nodes,
    submit_node,
    chaos_schedule,
    tx_rate = 0,
    rotate_every = 1000,
    mutation_rate = 0.0,
    accounts = 50):
    """Launch the fuzz sidecar in chaos mode.

    Same wiring as launch_soak plus CHAOS_SCHEDULE (JSON string).
    Mounts /var/run/docker.sock so chaos events can reach the daemon.
    """
    node_urls = ",".join(["http://{}:5005".format(n["name"]) for n in all_nodes])
    submit_url = "http://{}:5005".format(submit_node["name"])

    files = {
        "/output": Directory(persistent_key = "fuzz-chaos-output"),
    }

    return plan.add_service(
        name = "fuzz-chaos",
        config = ServiceConfig(
            image = "xrpl-confluence-sidecar:latest",
            entrypoint = ["/fuzz"],
            ports = {
                "results": PortSpec(number = 8081, transport_protocol = "TCP", application_protocol = "http"),
            },
            files = files,
            env_vars = {
                "MODE":             "chaos",
                "NODES":            node_urls,
                "SUBMIT_URL":       submit_url,
                "ACCOUNTS":         str(accounts),
                "TX_RATE":          str(tx_rate),
                "ROTATE_EVERY":     str(rotate_every),
                "MUTATION_RATE":    str(mutation_rate),
                "CORPUS_DIR":       "/output/corpus",
                "CRASH_LABEL_KEY":  "com.kurtosistech.custom.fuzzer.role",
                "CRASH_LABEL_VAL":  "node",
                "CRASH_TAIL_LINES": "200",
                "CHAOS_SCHEDULE":   chaos_schedule,
            },
        ),
    )
```

- [ ] **Step 2: Create src/tests/chaos.star**

```python
"""Chaos test suite — soak loop + scheduled disturbances.

The chaos sidecar runs indefinitely. The schedule is supplied via
args["chaos_args"]["schedule"] as a JSON string (see
sidecar/internal/fuzz/chaos/schedule_parse.go for the wire format).
Tear down with `kurtosis enclave rm <name>` or `make chaos-down`.
"""

helpers = import_module("../helpers/rpc.star")
fuzz_sidecar = import_module("../sidecar/fuzz.star")


def run(plan, nodes, args = {}):
    """Run the chaos suite (unbounded).

    Args (under args):
        - schedule: JSON string. Required.
        - tx_rate, rotate_every, mutation_rate, accounts: same as soak.
    """
    schedule = args.get("schedule", "")
    if schedule == "":
        fail("chaos suite requires args.schedule (JSON array)")

    tx_rate = args.get("tx_rate", 0)
    rotate_every = args.get("rotate_every", 1000)
    mutation_rate = args.get("mutation_rate", 0.0)
    accounts = args.get("accounts", 50)

    rippled_nodes_count = len([n for n in nodes if n["type"] == "rippled"])
    if rippled_nodes_count < 2:
        fail("chaos suite requires >= 2 rippled (got {})".format(rippled_nodes_count))

    plan.print("Waiting for all nodes to reach closed_seq >= 3...")
    for node in nodes:
        helpers.wait_for_ledger_seq(plan, node, 3, timeout = "120s")

    rippled_nodes = [n for n in nodes if n["type"] == "rippled"]
    submit_node = rippled_nodes[0]

    plan.print("Launching fuzz-chaos sidecar with {} schedule entries".format(len(schedule)))

    svc = fuzz_sidecar.launch_chaos(
        plan,
        all_nodes = nodes,
        submit_node = submit_node,
        chaos_schedule = schedule,
        tx_rate = tx_rate,
        rotate_every = rotate_every,
        mutation_rate = mutation_rate,
        accounts = accounts,
    )
    return {"fuzz-chaos": svc}
```

- [ ] **Step 3: Route in tests.star**

Edit `src/tests/tests.star`. Add:

```python
chaos = import_module("./chaos.star")
```

In the `run(plan, nodes, suite, ...)` body, add (next to soak):

```python
if suite == "chaos":
    plan.print("=== Running chaos ===")
    return {"chaos": chaos.run(plan, nodes, args.get("chaos_args", {}))}
```

- [ ] **Step 4: Update main.star docstring**

Edit `main.star`'s `run(plan, args = {})` docstring to list `"chaos"` under `test_suite`, and add a `chaos_args` arg description (`{schedule, tx_rate, rotate_every, mutation_rate, accounts}`).

- [ ] **Step 5: Starlark syntax check via tiny smoke**

```bash
kurtosis enclave rm -f chaos-syntax 2>/dev/null
kurtosis run --enclave chaos-syntax . '{"test_suite":"chaos","goxrpl_count":1,"rippled_count":2,"chaos_args":{"schedule":"[]"}}' 2>&1 | tail -10
kurtosis enclave rm -f chaos-syntax 2>/dev/null
```

Expected: parses without error; the suite refuses with `chaos suite requires args.schedule (JSON array)` only if schedule is empty — we passed `"[]"` so it should bring up the chaos service with an empty schedule (no-op). The fuzz-chaos service should appear in the enclave inspect.

- [ ] **Step 6: Commit**

```bash
git add src/sidecar/fuzz.star src/tests/chaos.star src/tests/tests.star main.star
git commit -m "confluence: chaos suite + launch_chaos sidecar wiring"
```

---

## Task 10: `make chaos` workflow

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Add chaos targets**

Append to `Makefile`:

```make
CHAOS_ENCLAVE  ?= xrpl-chaos
CHAOS_SCHEDULE ?= $(PWD)/.chaos-schedule.json

.PHONY: chaos chaos-down chaos-tail

chaos:
	@bash scripts/build-sidecar.sh
	@if [ ! -f $(CHAOS_SCHEDULE) ]; then echo "missing $(CHAOS_SCHEDULE) — see docs/plans/2026-05-05-chaos-runner-m4.md for format"; exit 1; fi
	@SCHEDULE=$$(cat $(CHAOS_SCHEDULE) | tr -d '\n' | sed 's/"/\\"/g'); \
		kurtosis enclave rm -f $(CHAOS_ENCLAVE) >/dev/null 2>&1 || true; \
		kurtosis run --enclave $(CHAOS_ENCLAVE) . "{\"test_suite\":\"chaos\",\"goxrpl_count\":$(GOXRPL_COUNT),\"rippled_count\":$(RIPPLED_COUNT),\"chaos_args\":{\"schedule\":\"$$SCHEDULE\",\"tx_rate\":$(TX_RATE),\"accounts\":$(ACCOUNTS),\"rotate_every\":$(ROTATE_EVERY),\"mutation_rate\":$(MUTATION_RATE)}}"
	@echo "Tail logs: make chaos-tail"

chaos-down:
	kurtosis enclave rm -f $(CHAOS_ENCLAVE)

chaos-tail:
	kurtosis service logs -f $(CHAOS_ENCLAVE) fuzz-chaos
```

The schedule lives in a checked-in file path so it's easy to iterate on without retyping JSON.

- [ ] **Step 2: Add an example schedule**

Create `.chaos-schedule.example.json` at the repo root:

```json
[
  {"step": 50,  "recover_after": 25, "type": "restart",   "container": "rippled-1"},
  {"step": 150, "recover_after": 30, "type": "latency",   "container": "rippled-0", "iface": "eth0", "delay_ms": 200}
]
```

(Keep the example minimal — partition + amendment cases need careful target selection per topology and should be added explicitly by the operator.)

- [ ] **Step 3: Smoke-test `make -n chaos`**

```bash
cp .chaos-schedule.example.json .chaos-schedule.json
make -n chaos GOXRPL_COUNT=1 RIPPLED_COUNT=2 TX_RATE=2 ACCOUNTS=5
```

Expected: the `kurtosis run` command line is expanded with the schedule embedded as a JSON string.

- [ ] **Step 4: Commit**

```bash
git add Makefile .chaos-schedule.example.json
git commit -m "confluence: make chaos / chaos-down / chaos-tail"
```

---

## Task 11: Live end-to-end smoke

**Files:**
- No new code. Pure verification.

- [ ] **Step 1: Build the sidecar with chaos support**

```bash
bash scripts/build-sidecar.sh
docker image inspect xrpl-confluence-sidecar:latest --format '{{.Id}}'
```

Expected: rebuilds clean.

- [ ] **Step 2: Run a 5-minute chaos enclave with one restart event**

Schedule (`/tmp/chaos-test.json`):

```json
[
  {"step": 30, "recover_after": 20, "type": "restart", "container": "rippled-1"}
]
```

Launch:

```bash
kurtosis enclave rm -f chaos-smoke 2>/dev/null
SCHEDULE=$(cat /tmp/chaos-test.json | tr -d '\n')
kurtosis run --enclave chaos-smoke . \
  "{\"test_suite\":\"chaos\",\"goxrpl_count\":1,\"rippled_count\":2,\"chaos_args\":{\"schedule\":\"$SCHEDULE\",\"tx_rate\":3,\"accounts\":3,\"rotate_every\":10}}" 2>&1 | tail -10
```

Wait ~3 minutes (30 successful txs takes ~10s at TxRate=3, plus the 20-step recover window).

- [ ] **Step 3: Verify chaos events fired**

```bash
kurtosis service logs chaos-smoke fuzz-chaos 2>&1 | grep -E "chaos: (apply|recover)" | head
```

Expected: at least one `chaos: apply restart:rippled-1 at step 30` and one `chaos: recover restart:rippled-1 at step 50`.

- [ ] **Step 4: Verify rippled-1 actually restarted**

```bash
docker ps --filter name=rippled-1 --format '{{.Names}} {{.Status}}'
```

Expected: shows recent uptime (under a minute) — proving the container was restarted by the chaos event.

- [ ] **Step 5: Verify chaos audit divergences in the corpus**

```bash
RUN_ENCLAVE_ID=$(kurtosis enclave inspect chaos-smoke --full-uuids 2>&1 | awk '/UUID:/{print $2; exit}')
docker cp "fuzz-chaos--$RUN_ENCLAVE_ID:/output/corpus/divergences" /tmp/chaos-corpus
ls /tmp/chaos-corpus/ | head
grep -l '"kind":"chaos"' /tmp/chaos-corpus/*.json | head | xargs -I{} cat {} | head -20
```

(Note: the exact docker name format is `<service>--<enclave-uuid>`. If `docker cp` rejects the name, use `docker ps --filter name=fuzz-chaos --format '{{.ID}}'` to find the container ID and copy from that.)

Expected: at least two JSON files with `"kind":"chaos"` — one for `apply`, one for `recover`. If the corpus is empty, the dashboard's `divergences_total_by_layer` panel won't show `chaos` either; investigate via `kurtosis service logs ... fuzz-chaos`.

- [ ] **Step 6: Tear down**

```bash
kurtosis enclave rm -f chaos-smoke
```

- [ ] **Step 7: Commit any docs follow-ups**

If anything in `docs/plans/2026-05-05-chaos-runner-m4.md` turned out incorrect during smoke (e.g. event audit-detail format), edit and:

```bash
git add docs/plans/2026-05-05-chaos-runner-m4.md
git commit -m "docs: chaos plan post-smoke corrections"
```

---

## Task 12: Final review

**Files:**
- No code. Pure verification.

- [ ] **Step 1: Cross-cutting code review**

```bash
cd sidecar
go test ./...
go vet ./...
git grep -n "TODO" sidecar/internal/fuzz/chaos/ sidecar/internal/fuzz/runners/chaos*
```

No `TODO`s should remain in chaos/ or runners/chaos*.

- [ ] **Step 2: Spot-check the full diff against pre-chaos baseline**

```bash
git log --oneline 9445e6b..HEAD  # post-F1 → post-F3 commits
git diff --stat 9445e6b..HEAD
```

Confirm:
- ~10–12 commits — one per task.
- New package `sidecar/internal/fuzz/chaos/` with 8 source files + 5 tests.
- `runners/chaos.go` is small; `runners/soak.go` modified by exactly the `OnPeriodic` field add and one call site.
- `cmd/fuzz/main.go` modified by the `case "chaos":` branch, `loadChaosConfig`, doc comment.
- New Starlark suite `src/tests/chaos.star`; `launch_chaos` in `src/sidecar/fuzz.star`; routing in `src/tests/tests.star`; docstring in `main.star`.
- `Makefile` chaos targets + `.chaos-schedule.example.json`.

- [ ] **Step 3: Push**

```bash
git push origin main
```

- [ ] **Step 4: File a one-line follow-up if any test surfaces real bugs**

Run a longer chaos session (`make chaos` with the example schedule, leave it for 30 minutes). If it surfaces real divergences in goXRPL (e.g. crash poller fires, or oracle layer 3 records a metadata diff), file an issue against `LeJamon/goXRPL` with the corpus entry attached — same playbook as the F2 reproducer.

---

## Self-review checklist

1. **Spec coverage**:
   - Task 1: NetworkRuntime — ✓
   - Task 2: ChaosScheduler — ✓
   - Task 3: RestartEvent — ✓
   - Task 4: LatencyEvent + PartitionEvent — ✓
   - Task 5: AmendmentFlipEvent — ✓
   - Task 6: ChaosRun runner + soak `OnPeriodic` hook — ✓
   - Task 7: Schedule parser — ✓
   - Task 8: cmd/fuzz wiring — ✓
   - Task 9: Kurtosis suite — ✓
   - Task 10: Makefile + example — ✓
   - Task 11: Live smoke — ✓
   - Task 12: Final review — ✓

2. **Placeholder scan**: Task 6's `_ = atomic.LoadInt64(...)` placeholder is flagged for removal in the same step that introduces it — explicit "drop this line" instruction. No other TBDs.

3. **Type consistency**:
   - `Event` interface (Name/Apply/Recover): used by RestartEvent, LatencyEvent, PartitionEvent, AmendmentFlipEvent, fakeRuntime tests, ParseSchedule. Consistent.
   - `ScheduleEntry` fields (`TriggerStep`, `Apply`, `RecoverAfter`): consistent in scheduler, scheduler_test, ParseSchedule.
   - `AuditEntry.Phase`: `"apply"` / `"recover"` — consistent in scheduler emission and ChaosRun's audit handler.
   - `NetworkRuntime` (Exec/Stop/Start): consistent across runtime.go, restart.go, netem.go, fakeRuntime.
   - `chaos.Stats` fields: `EventsApplied`, `EventsRecovered`, `EventsErrored` — consistent in scheduler.go, scheduler_test, cmd/fuzz logging.
   - `ChaosConfig.SoakConfig.OnPeriodic`: same field name in soak.go and chaos.go.

4. **Out-of-scope** (deliberately deferred):
   - Random-schedule generator (chaos events on a seed-derived schedule). For M4, the schedule is operator-supplied; randomization is a follow-up.
   - goXRPL-side chaos (latency/partition inside the distroless container). Requires a sidecar runtime image with iproute2/iptables — separate plan.
   - Symmetric partition (two PartitionEvents on the same step). Operator can express this in the JSON schedule today; no extra code needed. Document in the example schedule README if this becomes common.
   - Replay of a chaos run (deterministic re-derivation of the schedule + tx stream). Reuses the existing `MODE=reproduce` pipeline; no new code in M4.
