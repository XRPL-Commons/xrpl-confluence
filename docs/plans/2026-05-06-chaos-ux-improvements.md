# Chaos UX Improvements Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the chaos suite from "operator-stares-at-dashboard while babysitting a hand-authored event list" into a fire-and-forget multi-day soak with observability, alerting, recurring/wildcard event scheduling, reproducible runs, and a self-triaged corpus.

**Architecture:** Five independent phases, each shippable on its own.
- **Phase K** — Observability for the chaos suite (Starlark + Makefile, mirrors soak's existing wiring).
- **Phase L** — Recurring event generator + wildcard targeting in the chaos schedule (sidecar Go).
- **Phase M** — Alert webhook on first-seen divergence and on every crash (sidecar Go).
- **Phase N** — Reproducibility manifest written next to the corpus at startup (sidecar Go).
- **Phase O** — Corpus signature index + first-occurrence flagging (sidecar Go).

**Out of scope:** "goXRPL is still a passive observer" — that's a goXRPL passive-consensus bug (`goXRPL/internal/consensus/`), not a confluence-side issue. Confluence already has the wiring; until goXRPL fixes the bug, the chaos suite continues to use a rippled-only UNL (see `chaos.star:38` rationale). When the upstream fix lands, flipping `topology.star` to include goXRPL validators is one line.

**Tech Stack:** Go 1.24 (sidecar), Starlark (Kurtosis 1.x), Bash/Make, Prometheus, Grafana. No new Go dependencies.

**Repository roots:**
- `xrpl-confluence/` — working dir for all `git`/`make`/`kurtosis`/`go` commands.
- `xrpl-confluence/sidecar/internal/fuzz/{chaos,corpus,runners,metrics}/` — modified packages.
- `xrpl-confluence/sidecar/internal/fuzz/{alert,manifest}/` — new packages.
- `xrpl-confluence/src/{tests,sidecar}/` — Starlark.

**File structure:**

Phase K:
- `src/tests/chaos.star` — **modify.** Honor `args["enable_observability"]`; launch `prometheus` + `grafana` against `fuzz-chaos`.
- `Makefile` — **modify.** New `OBSERVABILITY ?=` flag, plumbed into the `chaos` target's JSON args.

Phase L:
- `sidecar/internal/fuzz/chaos/recurring.go` — **new.** `Recurring` event-spec struct + `ExpandRecurring(spec, env, rng) []rawEntry` that materialises many concrete entries.
- `sidecar/internal/fuzz/chaos/recurring_test.go` — **new.** Determinism, jitter, count/until_step, target-pattern resolution.
- `sidecar/internal/fuzz/chaos/schedule_parse.go` — **modify.** Accept `recurring` entry type; thread `ScheduleEnv` (node list + seed) through.
- `sidecar/internal/fuzz/chaos/schedule_parse_test.go` — **modify.** Cover the new wire shape.
- `sidecar/cmd/fuzz/main.go` — **modify.** Pass `ScheduleEnv{Nodes, Seed}` into `ParseSchedule`.

Phase M:
- `sidecar/internal/fuzz/alert/webhook.go` — **new.** `Webhook` poster (Slack/Discord-compatible JSON), in-memory dedup keyed by signature so we don't pager-spam.
- `sidecar/internal/fuzz/alert/webhook_test.go` — **new.** httptest-mocked round-trip + dedup behaviour.
- `sidecar/internal/fuzz/runners/realtime.go`, `soak.go`, `chaos.go` — **modify.** Each call site that records a divergence or crash also calls `alerter.Maybe(...)`.
- `sidecar/internal/fuzz/runners/realtime.go` (Config) — **modify.** Add `Alerter *alert.Webhook`.
- `sidecar/cmd/fuzz/main.go` — **modify.** Read `ALERT_WEBHOOK_URL` (+ optional `ALERT_LEVEL`) and construct a `Webhook` per run.

Phase N:
- `sidecar/internal/fuzz/manifest/manifest.go` — **new.** `Manifest` struct + `Write(corpusDir, m) error`. Captures every env-derived knob plus image versions and started-at.
- `sidecar/internal/fuzz/manifest/manifest_test.go` — **new.** Serialisation + idempotency.
- `sidecar/cmd/fuzz/main.go` — **modify.** Build a `Manifest` per mode and call `manifest.Write(...)` once before runner start.

Phase O:
- `sidecar/internal/fuzz/corpus/signature.go` — **modify.** Add `Signature(d *Divergence) DivergenceSignature` + `(s) Key() string` (filesystem-safe).
- `sidecar/internal/fuzz/corpus/divergence.go` — **modify.** `RecordDivergence` continues to write the time-keyed file in `divergences/`, **and additionally** writes a per-signature index entry under `signatures/<key>/` (`first.json`, `count.txt`). Returns `(isFirstSeen bool, err error)` so callers can hook alerts/triage.
- `sidecar/internal/fuzz/corpus/divergence_test.go` — **modify.** Cover the new index plus the first-seen return.
- `sidecar/internal/fuzz/runners/realtime.go`, `soak.go`, `chaos.go` — **modify.** Use the new return value; route first-seen through the alerter (Phase M dependency).
- `sidecar/internal/fuzz/metrics/metrics.go` — **modify.** New `UniqueSignatures` gauge driven from the per-signature index size.

---

## Phase K — Observability for chaos

### Task K1: Wire `enable_observability` into `src/tests/chaos.star`

**Files:**
- Modify: `src/tests/chaos.star`

`soak.star:85` already shows the pattern: `prometheus.launch(...)` + `grafana.launch(...)` gated on `args.get("enable_observability", False)`. Mirror it for chaos with `fuzz_service_name = "fuzz-chaos"`.

- [ ] **Step 1: Append observability block at the bottom of `chaos.star`'s `run()`**

Open `src/tests/chaos.star`. Right before `return {"fuzz-chaos": svc}` (currently the last line), insert:

```python
    if args.get("enable_observability", False):
        prom = import_module("../sidecar/prometheus.star")
        graf = import_module("../sidecar/grafana.star")
        prom.launch(plan, fuzz_service_name = "fuzz-chaos")
        graf.launch(plan, prometheus_service_name = "prometheus")
        plan.print("observability: prometheus on :9090, grafana on :3000 (anonymous viewer)")
```

The whitespace must be 4 spaces (Starlark) — match the surrounding indentation exactly.

- [ ] **Step 2: Verify the file parses**

```bash
kurtosis lint . 2>/dev/null || true
python3 -c "import ast; ast.parse(open('src/tests/chaos.star').read())"
```

Expected: no SyntaxError. (Starlark is not Python but the syntax is close enough that `ast.parse` catches obvious indentation/typos. Real validation happens in K3.)

### Task K2: Add `OBSERVABILITY` flag to the Makefile chaos target

**Files:**
- Modify: `Makefile`

The `chaos:` target currently builds the JSON args string with no observability key. Add a tri-state flag (default off, `1`/`true` turns it on) and inline it into the `chaos_args` JSON.

- [ ] **Step 1: Add `OBSERVABILITY` near the other top-of-file knobs**

In `Makefile`, after the `MUTATION_RATE ?= 0.05` line, add:

```make
OBSERVABILITY ?= 0
```

- [ ] **Step 2: Inject `enable_observability` into the chaos JSON args**

Replace the existing `chaos:` recipe body (the line that begins `kurtosis run --enclave $(CHAOS_ENCLAVE) ...`) with a version that includes the new key. Concretely: change the `chaos_args` object to include `,\"enable_observability\":$(OBSERVABILITY_BOOL)`. Add this helper line just above the recipe:

```make
OBSERVABILITY_BOOL := $(if $(filter 1 true yes,$(OBSERVABILITY)),true,false)
```

Then in the recipe, change the `chaos_args` JSON to:

```
"chaos_args":{"schedule":"$$SCHEDULE","tx_rate":$(TX_RATE),"accounts":$(ACCOUNTS),"rotate_every":$(ROTATE_EVERY),"mutation_rate":$(MUTATION_RATE),"enable_observability":$(OBSERVABILITY_BOOL)}
```

Apply the same change to the `soak:` recipe so both suites take the flag uniformly:

```
"soak_args":{"tx_rate":$(TX_RATE),"accounts":$(ACCOUNTS),"rotate_every":$(ROTATE_EVERY),"mutation_rate":$(MUTATION_RATE),"enable_observability":$(OBSERVABILITY_BOOL)}
```

- [ ] **Step 3: Smoke-expand both targets**

```bash
make -n chaos OBSERVABILITY=1 CHAOS_SCHEDULE=/tmp/nope.json 2>/dev/null | grep enable_observability || true
make -n soak  OBSERVABILITY=1 | grep enable_observability
```

Expected: each line includes `"enable_observability":true`.

```bash
make -n chaos CHAOS_SCHEDULE=/tmp/nope.json 2>/dev/null | grep enable_observability || true
make -n soak  | grep enable_observability
```

Expected: each line includes `"enable_observability":false`.

### Task K3: End-to-end smoke

**Files:** none.

- [ ] **Step 1: Build sidecar + tools image**

```bash
bash scripts/build-sidecar.sh
bash scripts/build-goxrpl-tools.sh
```

- [ ] **Step 2: Author a one-event schedule and launch chaos with observability on**

```bash
cat > .chaos-schedule.json <<'EOF'
[{"step": 50, "recover_after": 25, "type": "restart", "container": "rippled-1"}]
EOF
make docker-proxy
make chaos OBSERVABILITY=1 GOXRPL_COUNT=2 RIPPLED_COUNT=3 TX_RATE=2 ACCOUNTS=10 ROTATE_EVERY=200 MUTATION_RATE=0.0
```

- [ ] **Step 3: Confirm prometheus + grafana services exist in the enclave**

```bash
kurtosis enclave inspect xrpl-chaos | grep -E '^(prometheus|grafana|fuzz-chaos)'
```

Expected: three service rows. Note the `prometheus` and `grafana` IPs from `kurtosis service inspect`.

- [ ] **Step 4: Hit `/metrics` through prometheus**

```bash
PROM_IP=$(kurtosis service inspect xrpl-chaos prometheus | awk '/IP Address/ {print $3; exit}')
curl -s "http://$PROM_IP:9090/api/v1/query?query=up{job=\"fuzz\"}" | head -c 200; echo
```

Expected: a JSON envelope with `"status":"success"` and `"value":["...",1]` for the fuzz target.

- [ ] **Step 5: Tear down**

```bash
make chaos-down
```

- [ ] **Step 6: Commit Phase K**

```bash
git add src/tests/chaos.star Makefile
git commit -m "chaos: enable_observability launches prometheus + grafana (mirrors soak)"
```

---

## Phase L — Recurring event generator + wildcard targeting

The current schedule wire format (`sidecar/internal/fuzz/chaos/schedule_parse.go:12`) is a flat list of concrete `(step, type, container, ...)` entries. For week-long runs the operator wants:

- **recurring**: "every N steps fire event X" with optional `until_step` or `count` ceiling.
- **jitter**: ±k steps random offset around the periodic anchor (deterministic from `FUZZ_SEED`).
- **wildcard targeting**: `"container": "rippled-*"` resolves at expansion time to a random matching node from the run's `NODES` env.
- **range fields**: `delay_ms_min`/`delay_ms_max` for `latency`; `delay_ms` (singleton) still accepted.

Approach: a `recurring` entry expands at parse time into N concrete `rawEntry`s. Wildcards resolve against the node-name list extracted from the sidecar's `NODES` env (passed through a new `ScheduleEnv` parameter). Determinism comes from a child RNG seeded with `FUZZ_SEED` xor the entry's index.

### Task L1: Test for `Recurring.Expand` — basic step/count

**Files:**
- Create: `sidecar/internal/fuzz/chaos/recurring_test.go`

- [ ] **Step 1: Write the failing test**

Create `sidecar/internal/fuzz/chaos/recurring_test.go`:

```go
package chaos

import (
	"math/rand"
	"testing"
)

func TestExpandRecurring_FixedCount(t *testing.T) {
	spec := Recurring{
		Every:        100,
		Count:        3,
		StartStep:    50,
		Inner: rawEntry{
			Type:         "restart",
			Container:    "rippled-0",
			RecoverAfter: 10,
		},
	}
	env := ScheduleEnv{Nodes: []string{"rippled-0", "rippled-1"}, Seed: 1}
	out, err := ExpandRecurring(spec, env, rand.New(rand.NewSource(1)))
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("want 3 entries, got %d", len(out))
	}
	wantSteps := []int{50, 150, 250}
	for i, e := range out {
		if e.Step != wantSteps[i] {
			t.Errorf("entry %d: step %d, want %d", i, e.Step, wantSteps[i])
		}
		if e.Type != "restart" || e.Container != "rippled-0" || e.RecoverAfter != 10 {
			t.Errorf("entry %d: unexpected payload %+v", i, e)
		}
	}
}

func TestExpandRecurring_UntilStep(t *testing.T) {
	spec := Recurring{Every: 50, UntilStep: 175, StartStep: 0,
		Inner: rawEntry{Type: "restart", Container: "rippled-0", RecoverAfter: 5}}
	out, err := ExpandRecurring(spec, ScheduleEnv{Nodes: []string{"rippled-0"}}, rand.New(rand.NewSource(1)))
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if len(out) != 4 { // 0, 50, 100, 150
		t.Fatalf("want 4 entries, got %d", len(out))
	}
}
```

- [ ] **Step 2: Run; expect compile failure (Recurring/ExpandRecurring/ScheduleEnv don't exist yet)**

```bash
cd sidecar
go test ./internal/fuzz/chaos/... -run TestExpandRecurring -count=1
```

Expected: build error referencing `undefined: Recurring` etc.

### Task L2: Implement `Recurring`, `ScheduleEnv`, and `ExpandRecurring`

**Files:**
- Create: `sidecar/internal/fuzz/chaos/recurring.go`

- [ ] **Step 1: Add the file**

Create `sidecar/internal/fuzz/chaos/recurring.go`:

```go
package chaos

import (
	"fmt"
	"math/rand"
	"strings"
)

// ScheduleEnv carries information ParseSchedule needs to expand recurring/
// wildcard entries: the node-name universe (extracted from the sidecar's
// NODES env) and the run seed (used to derive a deterministic child RNG).
type ScheduleEnv struct {
	Nodes []string
	Seed  uint64
}

// Recurring is a higher-level entry type that materialises many concrete
// rawEntry values during ParseSchedule. Use it to express "every N steps,
// fire this event, optionally with jitter and a randomised target."
type Recurring struct {
	Every     int      `json:"every"`               // step interval; required, >0
	StartStep int      `json:"start_step"`          // default 0
	Count     int      `json:"count,omitempty"`     // optional cap on number of entries
	UntilStep int      `json:"until_step,omitempty"` // optional inclusive cap on step number
	Jitter    int      `json:"jitter,omitempty"`    // ±jitter steps applied per entry (deterministic)
	Inner     rawEntry `json:"event"`               // event-shaped fields; Step is ignored
}

// ExpandRecurring materialises the entries the recurring spec describes.
// rng is a child RNG; callers should seed it deterministically from the
// run seed plus the recurring entry's position.
func ExpandRecurring(spec Recurring, env ScheduleEnv, rng *rand.Rand) ([]rawEntry, error) {
	if spec.Every <= 0 {
		return nil, fmt.Errorf("recurring: every must be > 0")
	}
	if spec.Count <= 0 && spec.UntilStep <= 0 {
		return nil, fmt.Errorf("recurring: count or until_step required")
	}
	if spec.Inner.Type == "" {
		return nil, fmt.Errorf("recurring: event.type required")
	}

	out := []rawEntry{}
	for i := 0; ; i++ {
		step := spec.StartStep + i*spec.Every
		if spec.UntilStep > 0 && step > spec.UntilStep {
			break
		}
		if spec.Count > 0 && i >= spec.Count {
			break
		}
		entry := spec.Inner
		if spec.Jitter > 0 {
			step += rng.Intn(2*spec.Jitter+1) - spec.Jitter
			if step < 0 {
				step = 0
			}
		}
		entry.Step = step

		// Resolve wildcard targets ("rippled-*") against env.Nodes.
		if err := resolveWildcards(&entry, env, rng); err != nil {
			return nil, fmt.Errorf("recurring entry %d: %w", i, err)
		}

		// Optional latency range: delay_ms_min/delay_ms_max collapse into delay_ms.
		if entry.Type == "latency" && entry.DelayMs == 0 {
			lo, hi := spec.Inner.DelayMsMin, spec.Inner.DelayMsMax
			if hi > 0 && lo > 0 && hi >= lo {
				entry.DelayMs = lo + rng.Intn(hi-lo+1)
			}
		}

		out = append(out, entry)
	}
	return out, nil
}

func resolveWildcards(entry *rawEntry, env ScheduleEnv, rng *rand.Rand) error {
	for _, p := range []*string{&entry.Container, &entry.From, &entry.To, &entry.Target} {
		if !strings.Contains(*p, "*") {
			continue
		}
		match := matchNodes(*p, env.Nodes)
		if len(match) == 0 {
			return fmt.Errorf("no nodes match pattern %q (nodes=%v)", *p, env.Nodes)
		}
		*p = match[rng.Intn(len(match))]
	}
	return nil
}

func matchNodes(pattern string, nodes []string) []string {
	// Only suffix wildcard "*" supported (e.g. "rippled-*"). That covers the
	// realistic case; full glob would need filepath.Match semantics, which is
	// over-engineered for predictable Kurtosis names.
	if !strings.HasSuffix(pattern, "*") {
		return nil
	}
	prefix := strings.TrimSuffix(pattern, "*")
	out := []string{}
	for _, n := range nodes {
		if strings.HasPrefix(n, prefix) {
			out = append(out, n)
		}
	}
	return out
}
```

- [ ] **Step 2: Add `DelayMsMin`/`DelayMsMax` to `rawEntry`**

In `sidecar/internal/fuzz/chaos/schedule_parse.go`, extend the `rawEntry` struct:

```go
type rawEntry struct {
	Step         int    `json:"step"`
	RecoverAfter int    `json:"recover_after"`
	Type         string `json:"type"`
	Container    string `json:"container,omitempty"`
	From         string `json:"from,omitempty"`
	To           string `json:"to,omitempty"`
	Iface        string `json:"iface,omitempty"`
	DelayMs      int    `json:"delay_ms,omitempty"`
	DelayMsMin   int    `json:"delay_ms_min,omitempty"`
	DelayMsMax   int    `json:"delay_ms_max,omitempty"`
	Feature      string `json:"feature,omitempty"`
	Target       string `json:"target,omitempty"`

	// Recurring discriminator — when non-zero, the entry expands at parse
	// time and the other fields above (except Step which is ignored) are
	// the template for each materialised entry.
	Recurring *Recurring `json:"recurring,omitempty"`
}
```

- [ ] **Step 3: Run the tests; expect green**

```bash
cd sidecar
go test ./internal/fuzz/chaos/... -run TestExpandRecurring -count=1
```

Expected: PASS.

### Task L3: Wildcard + jitter test

**Files:**
- Modify: `sidecar/internal/fuzz/chaos/recurring_test.go`

- [ ] **Step 1: Append two more tests**

```go
func TestExpandRecurring_Wildcard(t *testing.T) {
	spec := Recurring{
		Every:     50,
		Count:     4,
		StartStep: 0,
		Inner: rawEntry{
			Type:         "restart",
			Container:    "rippled-*",
			RecoverAfter: 5,
		},
	}
	env := ScheduleEnv{Nodes: []string{"rippled-0", "rippled-1", "rippled-2", "goxrpl-0"}, Seed: 1}
	out, err := ExpandRecurring(spec, env, rand.New(rand.NewSource(1)))
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	for _, e := range out {
		if !strings.HasPrefix(e.Container, "rippled-") {
			t.Errorf("wildcard leaked non-rippled target: %q", e.Container)
		}
	}
	// Determinism: same seed, same expansion.
	out2, _ := ExpandRecurring(spec, env, rand.New(rand.NewSource(1)))
	for i := range out {
		if out[i].Container != out2[i].Container {
			t.Errorf("non-deterministic: entry %d %q vs %q", i, out[i].Container, out2[i].Container)
		}
	}
}

func TestExpandRecurring_LatencyRange(t *testing.T) {
	spec := Recurring{
		Every: 100, Count: 5, StartStep: 0,
		Inner: rawEntry{
			Type:         "latency",
			Container:    "rippled-0",
			Iface:        "eth0",
			DelayMsMin:   50,
			DelayMsMax:   500,
			RecoverAfter: 60,
		},
	}
	env := ScheduleEnv{Nodes: []string{"rippled-0"}}
	out, err := ExpandRecurring(spec, env, rand.New(rand.NewSource(7)))
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	for _, e := range out {
		if e.DelayMs < 50 || e.DelayMs > 500 {
			t.Errorf("delay_ms %d outside [50,500]", e.DelayMs)
		}
	}
}
```

Add `"strings"` to the imports of `recurring_test.go`.

- [ ] **Step 2: Run; expect green**

```bash
cd sidecar
go test ./internal/fuzz/chaos/... -run TestExpandRecurring -count=1
```

Expected: PASS.

### Task L4: Thread `Recurring` through `ParseSchedule`

**Files:**
- Modify: `sidecar/internal/fuzz/chaos/schedule_parse.go`
- Modify: `sidecar/internal/fuzz/chaos/schedule_parse_test.go` (or create if missing)
- Modify: `sidecar/cmd/fuzz/main.go`

- [ ] **Step 1: Update `ParseSchedule` signature to accept `ScheduleEnv`**

In `schedule_parse.go`, change:

```go
func ParseSchedule(raw string, rt NetworkRuntime) ([]ScheduleEntry, error) {
```

to:

```go
func ParseSchedule(raw string, rt NetworkRuntime, env ScheduleEnv) ([]ScheduleEntry, error) {
```

Inside the function, before the existing `for i, r := range entries` loop, insert recurring expansion. Replace the `for i, r := range entries` loop body's first lines with this expanded version of the loop:

```go
	expanded := make([]rawEntry, 0, len(entries))
	for i, r := range entries {
		if r.Type == "recurring" || r.Recurring != nil {
			spec := r.Recurring
			if spec == nil {
				return nil, fmt.Errorf("entry %d (recurring): recurring{} block required", i)
			}
			rng := rand.New(rand.NewSource(int64(env.Seed) ^ int64(i+1)))
			more, err := ExpandRecurring(*spec, env, rng)
			if err != nil {
				return nil, fmt.Errorf("entry %d: %w", i, err)
			}
			expanded = append(expanded, more...)
			continue
		}
		expanded = append(expanded, r)
	}
	out := make([]ScheduleEntry, 0, len(expanded))
	for i, r := range expanded {
```

(then continue with the existing per-entry switch statement, but reading from `expanded` instead of `entries`).

Also expand wildcard targets on plain (non-recurring) entries. Before the type switch, add:

```go
		if err := resolveWildcards(&r, env, rand.New(rand.NewSource(int64(env.Seed)^int64(i+1)))); err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
```

Add `"math/rand"` to the imports.

- [ ] **Step 2: Update `cmd/fuzz/main.go` to pass `ScheduleEnv`**

In `loadChaosConfig`, where `chaos.ParseSchedule(scheduleJSON, asInterface)` is called (line ~428), change to:

```go
	env := chaos.ScheduleEnv{Nodes: nodeNamesFromURLs(soak.NodeURLs), Seed: soak.Seed}
	schedule, parseErr := chaos.ParseSchedule(scheduleJSON, asInterface, env)
```

Add a small helper near the bottom of `main.go`:

```go
func nodeNamesFromURLs(urls []string) []string {
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		s := strings.TrimPrefix(u, "http://")
		s = strings.TrimPrefix(s, "https://")
		if i := strings.Index(s, ":"); i > 0 {
			s = s[:i]
		}
		out = append(out, s)
	}
	return out
}
```

- [ ] **Step 3: Add a `schedule_parse` test for the new path**

Create or append to `sidecar/internal/fuzz/chaos/schedule_parse_test.go`:

```go
func TestParseSchedule_RecurringExpands(t *testing.T) {
	raw := `[{
		"type": "recurring",
		"recurring": {
			"every": 100,
			"count": 3,
			"start_step": 50,
			"event": {"type": "restart", "container": "rippled-*", "recover_after": 5}
		}
	}]`
	env := ScheduleEnv{Nodes: []string{"rippled-0", "rippled-1"}, Seed: 42}
	entries, err := ParseSchedule(raw, nil, env)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("want 3 expanded entries, got %d", len(entries))
	}
	wantSteps := []int{50, 150, 250}
	for i, e := range entries {
		if e.TriggerStep != wantSteps[i] {
			t.Errorf("entry %d: step %d, want %d", i, e.TriggerStep, wantSteps[i])
		}
	}
}
```

- [ ] **Step 4: Run the package's tests**

```bash
cd sidecar
go test ./internal/fuzz/chaos/... -count=1
```

Expected: PASS. If a pre-existing `TestParseSchedule_*` test fails because of the new signature, update its call site to add `ScheduleEnv{}`.

- [ ] **Step 5: Build the whole sidecar to catch any remaining call sites**

```bash
cd sidecar
go build ./...
```

Expected: clean build.

### Task L5: Document the wire format and ship

**Files:**
- Modify: `Makefile` (comment block above the `chaos:` target)
- Modify: `.chaos-schedule.example.json` if present, else create

- [ ] **Step 1: Add a recurring-form example schedule**

Create or overwrite `.chaos-schedule.example.json`:

```json
[
  {"step": 50, "recover_after": 25, "type": "restart", "container": "rippled-1"},
  {
    "type": "recurring",
    "recurring": {
      "every": 600,
      "until_step": 12000,
      "jitter": 30,
      "event": {
        "type": "latency",
        "container": "rippled-*",
        "iface": "eth0",
        "delay_ms_min": 50,
        "delay_ms_max": 500,
        "recover_after": 120
      }
    }
  }
]
```

- [ ] **Step 2: Build sidecar, run a 5-min chaos smoke**

```bash
bash scripts/build-sidecar.sh
cp .chaos-schedule.example.json .chaos-schedule.json
make chaos GOXRPL_COUNT=2 RIPPLED_COUNT=3 TX_RATE=2 ACCOUNTS=10 ROTATE_EVERY=200
```

Wait ~3 min. Tail logs:

```bash
make chaos-tail | head -200
```

Expected: log lines `chaos: apply latency:rippled-N:Xms at step Y` for several different N.

- [ ] **Step 3: Tear down**

```bash
make chaos-down
```

- [ ] **Step 4: Commit**

```bash
git add sidecar/internal/fuzz/chaos/recurring.go \
        sidecar/internal/fuzz/chaos/recurring_test.go \
        sidecar/internal/fuzz/chaos/schedule_parse.go \
        sidecar/internal/fuzz/chaos/schedule_parse_test.go \
        sidecar/cmd/fuzz/main.go \
        .chaos-schedule.example.json
git commit -m "chaos: recurring event spec + wildcard target resolution"
```

---

## Phase M — Alert webhook on first-seen divergence and on every crash

Long-running chaos sees thousands of small divergences but only a handful of *new* signatures per day. Pager-quality alerting needs in-process dedup keyed by signature, plus a "fire always" path for crashes (every crash is interesting). Slack/Discord both accept the same `{"text": "..."}` JSON shape, so one webhook poster covers both.

### Task M1: `alert.Webhook` skeleton + dedup test

**Files:**
- Create: `sidecar/internal/fuzz/alert/webhook.go`
- Create: `sidecar/internal/fuzz/alert/webhook_test.go`

- [ ] **Step 1: Write the failing test**

Create `sidecar/internal/fuzz/alert/webhook_test.go`:

```go
package alert

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestWebhook_DedupBySignature(t *testing.T) {
	var posts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&posts, 1)
		var got map[string]any
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		if got["text"] == "" {
			t.Errorf("no text field in payload: %s", body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wh := NewWebhook(srv.URL)
	wh.Maybe("sig-A", "first A divergence")
	wh.Maybe("sig-A", "second A divergence (dup)")
	wh.Maybe("sig-B", "first B divergence")

	wh.Wait()
	if got := atomic.LoadInt32(&posts); got != 2 {
		t.Errorf("want 2 posts (sig-A first + sig-B first), got %d", got)
	}
}

func TestWebhook_AlwaysFiresWhenSignatureEmpty(t *testing.T) {
	var posts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&posts, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wh := NewWebhook(srv.URL)
	wh.Maybe("", "crash 1")
	wh.Maybe("", "crash 2")
	wh.Wait()

	if got := atomic.LoadInt32(&posts); got != 2 {
		t.Errorf("want 2 posts (no dedup when signature empty), got %d", got)
	}
}

func TestWebhook_NilSafe(t *testing.T) {
	var wh *Webhook
	wh.Maybe("sig", "anything") // must not panic
}
```

- [ ] **Step 2: Run; expect compile failure**

```bash
cd sidecar
go test ./internal/fuzz/alert/... -count=1
```

Expected: build error referencing `undefined: NewWebhook`.

- [ ] **Step 3: Implement `webhook.go`**

Create `sidecar/internal/fuzz/alert/webhook.go`:

```go
// Package alert posts notifications to a Slack/Discord-compatible webhook
// when first-seen divergences or crashes occur. In-process dedup keyed by
// signature prevents pager spam; an empty signature bypasses dedup so every
// crash fires.
package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

// Webhook posts JSON {"text": "..."} bodies to a Slack/Discord-compatible
// incoming webhook URL. Posts run in background goroutines so the caller
// (a fuzz loop) is never blocked by network latency. Wait blocks until all
// in-flight posts complete — call it before sidecar shutdown.
type Webhook struct {
	url    string
	client *http.Client

	mu   sync.Mutex
	seen map[string]bool
	wg   sync.WaitGroup
}

// NewWebhook returns nil when url is empty (so callers can ignore it).
func NewWebhook(url string) *Webhook {
	if url == "" {
		return nil
	}
	return &Webhook{
		url:    url,
		client: &http.Client{Timeout: 10 * time.Second},
		seen:   map[string]bool{},
	}
}

// Maybe posts text to the webhook unless this signature has already fired
// once. An empty signature bypasses dedup (used for crashes — every crash
// is interesting). Safe to call on a nil receiver.
func (w *Webhook) Maybe(signature, text string) {
	if w == nil {
		return
	}
	if signature != "" {
		w.mu.Lock()
		if w.seen[signature] {
			w.mu.Unlock()
			return
		}
		w.seen[signature] = true
		w.mu.Unlock()
	}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.post(text)
	}()
}

// Wait blocks until all in-flight posts have completed. Safe on nil.
func (w *Webhook) Wait() {
	if w == nil {
		return
	}
	w.wg.Wait()
}

func (w *Webhook) post(text string) {
	body, _ := json.Marshal(map[string]string{"text": text})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		log.Printf("alert: build request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.client.Do(req)
	if err != nil {
		log.Printf("alert: post: %v", err)
		return
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.Printf("alert: webhook returned %s", resp.Status)
	}
}
```

- [ ] **Step 4: Run; expect green**

```bash
cd sidecar
go test ./internal/fuzz/alert/... -count=1
```

Expected: PASS.

### Task M2: Wire the alerter into runners

**Files:**
- Modify: `sidecar/internal/fuzz/runners/realtime.go`
- Modify: `sidecar/internal/fuzz/runners/soak.go`
- Modify: `sidecar/internal/fuzz/runners/chaos.go`
- Modify: `sidecar/cmd/fuzz/main.go`

The wiring strategy: extend `Config` with an `Alerter *alert.Webhook` field; runners call `cfg.Alerter.Maybe(sigKey, text)` at every divergence-recording site and at the crash callback. The signature comes from `corpus.Signature(d).Key()` (introduced in Phase O — for now compute it inline).

Until Phase O lands the Signature helper, an interim shim:

```go
func sigKey(d *corpus.Divergence) string {
	switch d.Kind {
	case "tx_result", "metadata":
		t, _ := d.Details["tx_type"].(string)
		return d.Kind + ":" + t
	case "state_hash":
		return d.Kind
	case "invariant":
		v, _ := d.Details["invariant"].(string)
		return d.Kind + ":" + v
	case "crash":
		return "" // crashes always fire
	}
	return d.Kind
}
```

(In Phase O this collapses into a single helper; for now keep it private to `runners` and replace in O3.)

- [ ] **Step 1: Add the field + helper**

In `sidecar/internal/fuzz/runners/realtime.go`, extend the `Config` struct: add after `LocalSign bool`:

```go
	// Alerter, when non-nil, fires on first-seen divergence signature and on
	// every crash. Nil disables.
	Alerter *alert.Webhook
```

Add the import: `"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/alert"`.

Below `nodeName(...)` add the `sigKey` helper above.

- [ ] **Step 2: Call the alerter at each `RecordDivergence` site**

In `realtime.go`, after every `_ = rec.RecordDivergence(&corpus.Divergence{...})` block (there are four: tx_result, metadata, state_hash, invariant), add:

```go
		cfg.Alerter.Maybe(sigKey(d), fmt.Sprintf("[%s] %s", d.Kind, d.Description))
```

(where `d` is the `&corpus.Divergence{...}` literal — refactor each to assign to `d` first, then call both `RecordDivergence(d)` and `Alerter.Maybe(...)`).

In the crash `OnCrash` callback (line 92-108 of `realtime.go`), at the end add:

```go
			cfg.Alerter.Maybe("", fmt.Sprintf("crash: %s exited %d (%s)", e.Container, e.ExitCode, e.Kind))
```

- [ ] **Step 3: Mirror the same edits in `soak.go` and `chaos.go`**

`soak.go` has the same four divergence-record sites and the same `OnCrash` callback. Apply the same refactor: assign each divergence literal to `d`, then call `Alerter.Maybe`.

`chaos.go` records a `kind="chaos"` divergence in `OnAudit` (line 28-32). Since chaos audit events aren't really alert-worthy by themselves, gate them: only fire when `a.Error != ""`. After the existing `RecordDivergence` call:

```go
		if a.Error != "" {
			cfg.Alerter.Maybe("chaos:"+a.Event, fmt.Sprintf("chaos event errored: %s/%s step %d: %s", a.Event, a.Phase, a.Step, a.Error))
		}
```

- [ ] **Step 4: Construct the alerter in `cmd/fuzz/main.go`**

In `loadConfig` (after the `LOCAL_SIGN` block ~line 268), append:

```go
	if u := os.Getenv("ALERT_WEBHOOK_URL"); u != "" {
		cfg.Alerter = alert.NewWebhook(u)
	}
```

Add the import: `"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/alert"`.

- [ ] **Step 5: Build and run package tests**

```bash
cd sidecar
go build ./...
go test ./internal/fuzz/runners/... -count=1
go test ./internal/fuzz/alert/... -count=1
```

Expected: all green. If `runners` tests reference a config with no Alerter field they'll keep working — `nil` is the safe default.

### Task M3: Plumb `ALERT_WEBHOOK_URL` through Starlark

**Files:**
- Modify: `src/sidecar/fuzz.star`
- Modify: `Makefile`

- [ ] **Step 1: Pass-through env var in `launch_soak` and `launch_chaos`**

In `src/sidecar/fuzz.star`, both `launch_soak` and `launch_chaos` build an `env_vars` dict. Add these arguments to the function signature (default empty string):

```python
        alert_webhook_url = "",
```

And inside `env_vars`, conditionally include:

```python
    env_vars = {
        # ...existing keys...
    }
    if alert_webhook_url != "":
        env_vars["ALERT_WEBHOOK_URL"] = alert_webhook_url
```

Then in `src/tests/soak.star` and `src/tests/chaos.star`, pull `args.get("alert_webhook_url", "")` and pass it through to `fuzz_sidecar.launch_soak(...)` / `launch_chaos(...)`.

- [ ] **Step 2: Makefile knob**

In `Makefile`, near `OBSERVABILITY ?= 0`, add:

```make
ALERT_WEBHOOK_URL ?=
```

In both `chaos:` and `soak:` recipes, append `,\"alert_webhook_url\":\"$(ALERT_WEBHOOK_URL)\"` to the JSON args.

Verify expansion:

```bash
make -n soak ALERT_WEBHOOK_URL=https://example.invalid/hook | grep alert_webhook_url
```

Expected: the URL is in the JSON.

- [ ] **Step 3: Commit Phase M**

```bash
git add sidecar/internal/fuzz/alert/ \
        sidecar/internal/fuzz/runners/realtime.go \
        sidecar/internal/fuzz/runners/soak.go \
        sidecar/internal/fuzz/runners/chaos.go \
        sidecar/cmd/fuzz/main.go \
        src/sidecar/fuzz.star \
        src/tests/soak.star \
        src/tests/chaos.star \
        Makefile
git commit -m "alert: webhook on first-seen divergence and on every crash"
```

---

## Phase N — Reproducibility manifest

A 5-day-old divergence is only reproducible if you can recreate the exact run config. Today, only `FUZZ_SEED` is logged. Drop a `run-manifest.json` next to `corpus/` capturing every env-derived knob plus image versions and start time.

### Task N1: Manifest type + Write

**Files:**
- Create: `sidecar/internal/fuzz/manifest/manifest.go`
- Create: `sidecar/internal/fuzz/manifest/manifest_test.go`

- [ ] **Step 1: Test**

Create `sidecar/internal/fuzz/manifest/manifest_test.go`:

```go
package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWrite_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := Manifest{
		Mode:       "chaos",
		Seed:       42,
		Accounts:   50,
		TxRate:     5,
		Mutation:   0.05,
		Rotate:     1000,
		LocalSign:  false,
		Nodes:      []string{"http://rippled-0:5005", "http://rippled-1:5005"},
		SubmitURL:  "http://rippled-0:5005",
		TierWeights: map[string]int{"rich": 1, "at_reserve": 0},
		Schedule:   "[{\"step\":50,\"type\":\"restart\",\"container\":\"rippled-1\"}]",
	}
	if err := Write(dir, m); err != nil {
		t.Fatalf("write: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "run-manifest.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var back Manifest
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Seed != 42 || back.Mode != "chaos" || back.StartedAt.IsZero() {
		t.Errorf("manifest round-trip lost data: %+v", back)
	}
}

func TestWrite_MkdirParents(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "deep", "path")
	if err := Write(dir, Manifest{Mode: "soak", Seed: 1}); err != nil {
		t.Fatalf("write: %v", err)
	}
}
```

- [ ] **Step 2: Implementation**

Create `sidecar/internal/fuzz/manifest/manifest.go`:

```go
// Package manifest writes a single run-manifest.json next to a fuzz corpus
// so that a divergence saved days later can be re-driven with identical
// config (seed, account count, tier weights, mutation rate, schedule, etc).
package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Manifest captures every knob that influences a fuzz run.
// Anything not reproducible from the run-manifest plus FUZZ_SEED should be
// added here.
type Manifest struct {
	StartedAt   time.Time      `json:"started_at"`
	Mode        string         `json:"mode"`
	Seed        uint64         `json:"seed"`
	Accounts    int            `json:"accounts"`
	TxCount     int            `json:"tx_count,omitempty"`
	TxRate      float64        `json:"tx_rate,omitempty"`
	Rotate      int64          `json:"rotate_every,omitempty"`
	Mutation    float64        `json:"mutation_rate,omitempty"`
	LocalSign   bool           `json:"local_sign"`
	Nodes       []string       `json:"node_urls"`
	SubmitURL   string         `json:"submit_url"`
	CorpusDir   string         `json:"corpus_dir"`
	BatchClose  string         `json:"batch_close,omitempty"`
	TierWeights map[string]int `json:"tier_weights,omitempty"`
	Schedule    string         `json:"chaos_schedule,omitempty"`
	Image       string         `json:"image,omitempty"` // sidecar image tag
	GitSHA      string         `json:"git_sha,omitempty"`
}

// Write serialises m to <corpusDir>/run-manifest.json. StartedAt is set to
// time.Now() if zero. Parent directories are created.
func Write(corpusDir string, m Manifest) error {
	if m.StartedAt.IsZero() {
		m.StartedAt = time.Now().UTC()
	}
	if err := os.MkdirAll(corpusDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", corpusDir, err)
	}
	path := filepath.Join(corpusDir, "run-manifest.json")
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}
```

- [ ] **Step 3: Run; expect green**

```bash
cd sidecar
go test ./internal/fuzz/manifest/... -count=1
```

Expected: PASS.

### Task N2: Wire the manifest into `cmd/fuzz/main.go`

**Files:**
- Modify: `sidecar/cmd/fuzz/main.go`

- [ ] **Step 1: Build a manifest at the start of each mode dispatch**

Add the import: `"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/manifest"`.

Add a helper above `main`:

```go
func writeManifest(mode string, m manifest.Manifest, corpusDir string) {
	m.Mode = mode
	m.CorpusDir = corpusDir
	m.Image = os.Getenv("FUZZ_IMAGE_TAG")
	m.GitSHA = os.Getenv("FUZZ_GIT_SHA")
	if err := manifest.Write(corpusDir, m); err != nil {
		log.Printf("manifest: %v", err)
	}
}
```

In each `case` branch in `main()` (`fuzz`, `replay`, `reproduce`, `shrink`, `soak`, `chaos`), immediately after the config is loaded and before `Run/SoakRun/...` is called, build a Manifest and write it. Example for `fuzz`:

```go
		writeManifest("fuzz", manifest.Manifest{
			Seed:      cfg.Seed,
			Accounts:  cfg.AccountN,
			TxCount:   cfg.TxCount,
			Mutation:  cfg.MutationRate,
			LocalSign: cfg.LocalSign,
			Nodes:     cfg.NodeURLs,
			SubmitURL: cfg.SubmitURL,
			BatchClose: cfg.BatchClose.String(),
			TierWeights: map[string]int{
				"rich":        cfg.TierWeights.Rich,
				"at_reserve":  cfg.TierWeights.AtReserve,
				"multisig":    cfg.TierWeights.Multisig,
				"regular_key": cfg.TierWeights.RegularKey,
				"blackholed":  cfg.TierWeights.Blackholed,
			},
		}, cfg.CorpusDir)
```

For `soak` and `chaos`, also set `TxRate`, `Rotate`, and (chaos only) `Schedule` from `os.Getenv("CHAOS_SCHEDULE")`. For `replay`, set `LedgerStart`/`LedgerEnd` — extend `Manifest` with these optional fields:

```go
	LedgerStart int    `json:"ledger_start,omitempty"`
	LedgerEnd   int    `json:"ledger_end,omitempty"`
```

(add now in `manifest.go`).

- [ ] **Step 2: Plumb image tag + git SHA through Starlark**

In `src/sidecar/fuzz.star`'s `launch_soak` and `launch_chaos`, set two extra env vars unconditionally:

```python
        "FUZZ_IMAGE_TAG":   "xrpl-confluence-sidecar:latest",
        "FUZZ_GIT_SHA":     "",  # plumbed by the build script later if needed
```

And in `scripts/build-sidecar.sh` add a `--build-arg GIT_SHA=$(git rev-parse HEAD)` so the image labels can record it (optional polish; if `build-sidecar.sh` doesn't do build args today, defer this).

- [ ] **Step 3: Build sidecar; quick unit run**

```bash
cd sidecar
go build ./...
go test ./internal/fuzz/manifest/... -count=1
```

Expected: clean build + green tests.

### Task N3: Smoke-verify the manifest appears

**Files:** none.

- [ ] **Step 1: Quick soak run**

```bash
bash scripts/build-sidecar.sh
make soak GOXRPL_COUNT=2 RIPPLED_COUNT=3 TX_RATE=2 ACCOUNTS=10 ROTATE_EVERY=200
sleep 30
make soak-pull
```

- [ ] **Step 2: Confirm `run-manifest.json` is present**

```bash
ls .soak-corpus/corpus/run-manifest.json && cat .soak-corpus/corpus/run-manifest.json | head -40
```

Expected: file exists and contains seed, accounts, tier_weights, etc.

- [ ] **Step 3: Tear down**

```bash
make soak-down
```

- [ ] **Step 4: Commit Phase N**

```bash
git add sidecar/internal/fuzz/manifest/ sidecar/cmd/fuzz/main.go src/sidecar/fuzz.star
git commit -m "fuzz: write run-manifest.json next to corpus on startup"
```

---

## Phase O — Corpus signature index + first-occurrence flagging

`Recorder.RecordDivergence` writes one timestamp-keyed JSON per divergence into `corpus/divergences/`. After 24h that's a haystack. Add a parallel `corpus/signatures/<key>/` index where `<key>` is the divergence's signature: `first.json` (the canonical specimen — exactly one per signature), `count.txt` (incremented on every match). Return `(isFirstSeen bool, err error)` from `RecordDivergence` so the alerter can use signature-aware dedup.

### Task O1: Add `Signature(d)` and `(s) Key()`

**Files:**
- Modify: `sidecar/internal/fuzz/corpus/signature.go`
- Modify: `sidecar/internal/fuzz/corpus/signature_test.go`

- [ ] **Step 1: Test**

Append to `sidecar/internal/fuzz/corpus/signature_test.go` (or create if absent):

```go
func TestSignature_FromDivergence(t *testing.T) {
	cases := []struct {
		name string
		d    Divergence
		want string // expected Key()
	}{
		{"tx_result", Divergence{Kind: "tx_result", Details: map[string]any{"tx_type": "Payment"}}, "tx_result_Payment"},
		{"metadata", Divergence{Kind: "metadata", Details: map[string]any{"tx_type": "OfferCreate"}}, "metadata_OfferCreate"},
		{"invariant", Divergence{Kind: "invariant", Details: map[string]any{"invariant": "pool_balance_monotone"}}, "invariant_pool_balance_monotone"},
		{"crash", Divergence{Kind: "crash"}, "crash"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Signature(&c.d).Key()
			if got != c.want {
				t.Errorf("Key()=%q want %q", got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Implement `Signature` and `Key`**

Add to `sidecar/internal/fuzz/corpus/signature.go`:

```go
// Signature derives a signature directly from an in-memory Divergence.
// Mirrors LoadDivergenceSignature's logic without disk IO.
func Signature(d *Divergence) DivergenceSignature {
	var s DivergenceSignature
	if d == nil {
		return s
	}
	s.Kind = d.Kind
	switch d.Kind {
	case "tx_result", "metadata":
		if v, ok := d.Details["tx_type"].(string); ok {
			s.TxType = v
		}
	case "state_hash":
		s.Field = stateHashField(d.Details)
	case "invariant":
		if v, ok := d.Details["invariant"].(string); ok {
			s.Invariant = v
		}
	}
	return s
}

// Key returns a filesystem-safe stable identifier for this signature.
// Used as the per-signature index directory name.
func (s DivergenceSignature) Key() string {
	parts := []string{s.Kind}
	for _, p := range []string{s.TxType, s.Field, s.Invariant} {
		if p != "" {
			parts = append(parts, sanitize(p))
		}
	}
	return strings.Join(parts, "_")
}

func sanitize(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_', c == '.':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}
```

Add `"strings"` to the imports.

- [ ] **Step 3: Run**

```bash
cd sidecar
go test ./internal/fuzz/corpus/... -run TestSignature -count=1
```

Expected: PASS.

### Task O2: `RecordDivergence` returns `isFirstSeen` and writes the index

**Files:**
- Modify: `sidecar/internal/fuzz/corpus/divergence.go`
- Modify: `sidecar/internal/fuzz/corpus/divergence_test.go`

- [ ] **Step 1: Test**

Append to `sidecar/internal/fuzz/corpus/divergence_test.go`:

```go
func TestRecordDivergence_SignatureIndex(t *testing.T) {
	dir := t.TempDir()
	rec := NewRecorder(dir, 1)

	first, err := rec.RecordDivergence(&Divergence{Kind: "tx_result", Details: map[string]any{"tx_type": "Payment"}})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if !first {
		t.Errorf("want isFirstSeen=true on first record")
	}

	again, err := rec.RecordDivergence(&Divergence{Kind: "tx_result", Details: map[string]any{"tx_type": "Payment"}})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if again {
		t.Errorf("want isFirstSeen=false on second record")
	}

	idx := filepath.Join(dir, "signatures", "tx_result_Payment")
	if _, err := os.Stat(filepath.Join(idx, "first.json")); err != nil {
		t.Errorf("missing first.json: %v", err)
	}
	cb, err := os.ReadFile(filepath.Join(idx, "count.txt"))
	if err != nil {
		t.Fatalf("count.txt: %v", err)
	}
	if got := strings.TrimSpace(string(cb)); got != "2" {
		t.Errorf("count.txt = %q, want 2", got)
	}
}
```

Add imports as needed (`"path/filepath"`, `"strings"`, `"os"`).

- [ ] **Step 2: Update the implementation**

In `sidecar/internal/fuzz/corpus/divergence.go`, change `RecordDivergence` to return `(bool, error)` and write the index:

```go
// RecordDivergence writes one divergence JSON file under
// `<baseDir>/divergences/<timestamp>_<counter>.json` and updates the per-
// signature index under `<baseDir>/signatures/<key>/`. Returns isFirstSeen
// = true if this signature has not been recorded before in this Recorder's
// lifetime, false otherwise. Filename includes a monotonic counter so
// concurrent callers never collide.
func (r *Recorder) RecordDivergence(d *Divergence) (bool, error) {
	if d.Seed == 0 {
		d.Seed = r.seed
	}
	if d.RecordedAt.IsZero() {
		d.RecordedAt = time.Now().UTC()
	}

	dir := filepath.Join(r.baseDir, "divergences")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, fmt.Errorf("mkdir %s: %w", dir, err)
	}

	n := r.counter.Add(1)
	name := fmt.Sprintf("%s_%06d.json", d.RecordedAt.Format("20060102T150405.000000"), n)
	path := filepath.Join(dir, name)

	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshal divergence: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}

	first, err := r.updateSignatureIndex(d, data)
	if err != nil {
		// Index failure must not lose the divergence file we already wrote.
		// Log via caller — return the err alongside firstSeen=false.
		return false, fmt.Errorf("signature index: %w", err)
	}
	return first, nil
}

func (r *Recorder) updateSignatureIndex(d *Divergence, data []byte) (bool, error) {
	key := Signature(d).Key()
	if key == "" {
		return false, nil
	}
	idx := filepath.Join(r.baseDir, "signatures", key)
	if err := os.MkdirAll(idx, 0o755); err != nil {
		return false, err
	}
	firstPath := filepath.Join(idx, "first.json")
	first := false
	if _, err := os.Stat(firstPath); os.IsNotExist(err) {
		if err := os.WriteFile(firstPath, data, 0o644); err != nil {
			return false, err
		}
		first = true
	}
	countPath := filepath.Join(idx, "count.txt")
	count := 0
	if cb, err := os.ReadFile(countPath); err == nil {
		fmt.Sscanf(strings.TrimSpace(string(cb)), "%d", &count)
	}
	count++
	if err := os.WriteFile(countPath, []byte(fmt.Sprintf("%d\n", count)), 0o644); err != nil {
		return false, err
	}
	return first, nil
}
```

Add `"strings"` to imports.

- [ ] **Step 3: Update existing callers**

Every call site of `_ = rec.RecordDivergence(...)` now must capture two return values. There are call sites in:
- `sidecar/internal/fuzz/runners/realtime.go` (4 sites: tx_result, metadata, state_hash, invariant)
- `sidecar/internal/fuzz/runners/realtime.go` `OnCrash` callback
- `sidecar/internal/fuzz/runners/soak.go` (4 sites + crash callback)
- `sidecar/internal/fuzz/runners/chaos.go` `OnAudit` (1 site)

Replace `_ = rec.RecordDivergence(d)` with `firstSeen, _ := rec.RecordDivergence(d)`. Use `firstSeen` to gate the `Alerter.Maybe(...)` call from Phase M:

```go
		firstSeen, _ := rec.RecordDivergence(d)
		if firstSeen {
			cfg.Alerter.Maybe(corpus.Signature(d).Key(), fmt.Sprintf("[%s] %s", d.Kind, d.Description))
		}
```

For `OnCrash` (no signature dedup; alert always), keep:

```go
			cfg.Alerter.Maybe("", fmt.Sprintf("crash: %s exited %d (%s)", e.Container, e.ExitCode, e.Kind))
```

You can now delete the interim `sigKey` shim added in Phase M2 — that helper was only needed before `corpus.Signature` existed.

- [ ] **Step 4: Run all sidecar tests**

```bash
cd sidecar
go test ./... -count=1
```

Expected: PASS. The only change in semantics is the new return type; all existing call sites must use the two-value form.

### Task O3: Metric for unique signatures

**Files:**
- Modify: `sidecar/internal/fuzz/metrics/metrics.go`
- Modify: `sidecar/internal/fuzz/runners/{realtime,soak}.go` (the periodic block)

- [ ] **Step 1: Add the gauge**

In `metrics.go`, find the existing `Registry` struct and add:

```go
	UniqueSignatures prometheus.Gauge
```

In `New()`, register it:

```go
	r.UniqueSignatures = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "fuzz_unique_signatures",
		Help: "Distinct divergence signatures observed in the current run.",
	})
	r.reg.MustRegister(r.UniqueSignatures)
```

(Match the surrounding style — counter vs gauge constructor names per the existing file.)

- [ ] **Step 2: Update the gauge from the periodic block**

In both `realtime.go` and `soak.go`, the periodic block already updates `Metrics.CorpusSize` from `os.ReadDir(filepath.Join(cfg.CorpusDir, "divergences"))`. Add right next to it:

```go
				if entries, err := os.ReadDir(filepath.Join(cfg.CorpusDir, "signatures")); err == nil {
					cfg.Metrics.UniqueSignatures.Set(float64(len(entries)))
				}
```

- [ ] **Step 3: Build, test, smoke**

```bash
cd sidecar
go build ./...
go test ./... -count=1
```

Expected: green.

- [ ] **Step 4: Commit Phase O**

```bash
git add sidecar/internal/fuzz/corpus/ \
        sidecar/internal/fuzz/runners/ \
        sidecar/internal/fuzz/metrics/metrics.go
git commit -m "corpus: signature index + first-seen return; alerter consumes it"
```

---

## End-to-end smoke (all phases)

### Task Z1: Multi-day-style smoke

**Files:** none.

- [ ] **Step 1: Compose a recurring schedule with wildcard targets**

```bash
cat > .chaos-schedule.json <<'EOF'
[
  {
    "type": "recurring",
    "recurring": {
      "every": 200,
      "until_step": 4000,
      "jitter": 30,
      "event": {
        "type": "latency",
        "container": "rippled-*",
        "iface": "eth0",
        "delay_ms_min": 50,
        "delay_ms_max": 500,
        "recover_after": 100
      }
    }
  },
  {
    "type": "recurring",
    "recurring": {
      "every": 800,
      "until_step": 4000,
      "event": {"type": "restart", "container": "rippled-*", "recover_after": 30}
    }
  }
]
EOF
```

- [ ] **Step 2: Launch with everything on**

```bash
bash scripts/build-sidecar.sh
bash scripts/build-goxrpl-tools.sh
make docker-proxy
make chaos OBSERVABILITY=1 ALERT_WEBHOOK_URL=https://example.invalid/hook \
     GOXRPL_COUNT=2 RIPPLED_COUNT=3 TX_RATE=3 ACCOUNTS=20 ROTATE_EVERY=300 MUTATION_RATE=0.05
```

`https://example.invalid/hook` will fail to deliver but the alerter logs the failure rather than panicking — verifying the wiring without spamming a real Slack.

- [ ] **Step 3: Verify**

After ~5 minutes:

```bash
make chaos-pull
ls .chaos-corpus/corpus/run-manifest.json   # Phase N
ls .chaos-corpus/corpus/signatures/         # Phase O
make chaos-tail | grep -E 'apply (latency|restart)' | head -10  # Phase L
PROM_IP=$(kurtosis service inspect xrpl-chaos prometheus | awk '/IP Address/ {print $3; exit}')
curl -s "http://$PROM_IP:9090/api/v1/query?query=fuzz_unique_signatures" | head -c 200; echo  # Phase O metric
make chaos-tail | grep 'alert: post' | head -3  # Phase M (errors expected from invalid URL)
```

Each line should produce evidence: file present, multiple distinct latency/restart names in logs, prometheus returns a numeric value, and the alerter logs (failed) post attempts.

- [ ] **Step 4: Tear down**

```bash
make chaos-down
docker rm -f xrpl-confluence-docker-proxy 2>/dev/null || true
```

---

## Self-review

1. **Spec coverage:**
   - #1 (observability) → Phase K ✓
   - #2 (schedule DSL/recurring/wildcard) → Phase L ✓
   - #3 (alerting) → Phase M ✓
   - #4 (manifest) → Phase N ✓
   - #5 (goXRPL passive consensus) → explicitly out of scope, documented in plan header ✓
   - #6 (corpus triage) → Phase O ✓ (note: auto-shrink-on-first-seen is deferred — Phase O writes a structured `signatures/<key>/first.json` index that an external cron or follow-up task can feed to the existing `MODE=shrink` runner; spawning shrink in-process would couple two runners and is rightly a separate phase).

2. **Type consistency:** `Recurring` (Phase L) → `ParseSchedule(raw, rt, env)` (L4) → `cmd/fuzz/main.go` `nodeNamesFromURLs` helper. `Webhook.Maybe(signature, text)` (M1) → all call sites pass `corpus.Signature(d).Key()` once Phase O lands; the interim `sigKey` shim in M2 is deleted in O3 step 3. `RecordDivergence` returns `(bool, error)` from O2 onward — every call site updated in O2 step 3.

3. **Placeholder scan:** Each TDD step contains the actual test code. Each implementation step contains the actual code. Smoke commands include exact expected output. No "TBD" or "fill in details."

---

## Execution Handoff

**Plan complete and saved to `docs/plans/2026-05-06-chaos-ux-improvements.md`.**

The user asked me to write a plan **and work on it** in the same turn. I'll execute inline using `superpowers:executing-plans`, batching tasks per phase with checkpoints between phases so the user can interrupt or course-correct.
