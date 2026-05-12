# Agent-Friendly Interface M1 — API Contract + Scenarios

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the typed contract everything else builds on — `api.Scenario`, `api.Finding`, `api.Error` types; scenario load/validate/compile-to-Kurtosis-args; a `confluence` CLI binary with one working subcommand (`scenario validate`). No server, no Kurtosis interaction yet.

**Architecture:** All new code lives inside the existing `sidecar/` Go module (per design doc). Three new internal packages — `api` (types), `scenario` (load/validate/compile), `finding` (ID helpers; full store comes in M2) — plus a Cobra-based `cmd/confluence` CLI. TDD throughout: every behavior gets a failing test first.

**Tech Stack:** Go 1.25, `github.com/spf13/cobra` (CLI), `gopkg.in/yaml.v3` (scenario YAML), `github.com/oklog/ulid/v2` (IDs). All work runs from `sidecar/` unless noted.

**Reference:** `docs/design/2026-05-12-agent-friendly-interface-design.md`

**Scope:**
- In: `api` types (Scenario, Finding, Error, APIVersion constant); `scenario.Load`, `scenario.Validate`, `scenario.Compile`; `finding.NewFindingID/NewRunID/NewReproducerID`; `confluence version` and `confluence scenario validate` subcommands; one built-in scenario (`scenarios/soak-mixed-3x2.yaml`); a golden-file compile test that locks the Kurtosis args shape against today's `Makefile` output.
- Out (deferred to M2+): HTTP server, finding store, `confluence up/run/findings/...`, dashboard migration, Makefile delegation, `agents.md`.

---

### Task 1: Add dependencies and a minimal CLI binary

**Files:**
- Modify: `sidecar/go.mod`, `sidecar/go.sum`
- Create: `sidecar/cmd/confluence/main.go`
- Create: `sidecar/cmd/confluence/version.go`
- Create: `sidecar/cmd/confluence/main_test.go`

- [ ] **Step 1: Add the new dependencies**

Run from `sidecar/`:

```bash
go get github.com/spf13/cobra@latest
go get gopkg.in/yaml.v3@latest
go get github.com/oklog/ulid/v2@latest
go mod tidy
```

Expected: `go.mod` gains all three direct deps; `go.sum` populated; `go build ./...` still succeeds (`go build ./...` exit 0).

- [ ] **Step 2: Write a failing test that the binary prints version JSON**

Create `sidecar/cmd/confluence/main_test.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestVersionJSON(t *testing.T) {
	out := &bytes.Buffer{}
	root := newRootCmd()
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs([]string{"version", "--json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var got struct {
		Version    string `json:"version"`
		APIVersion string `json:"api_version"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("not JSON: %v (out=%q)", err, out.String())
	}
	if got.APIVersion != "confluence/v1" {
		t.Fatalf("api_version: got %q want %q", got.APIVersion, "confluence/v1")
	}
	if got.Version == "" {
		t.Fatalf("version: empty")
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./cmd/confluence/...`
Expected: FAIL with "no Go files" or "undefined: newRootCmd".

- [ ] **Step 4: Create the CLI root**

Create `sidecar/cmd/confluence/main.go`:

```go
// Package main is the confluence CLI entrypoint.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "confluence",
		Short:         "Drive xrpl-confluence enclaves",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().Bool("json", false, "Emit machine-readable JSON on stdout")
	root.AddCommand(newVersionCmd())
	return root
}
```

- [ ] **Step 5: Create the version subcommand**

Create `sidecar/cmd/confluence/version.go`:

```go
package main

import (
	"encoding/json"
	"fmt"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
	"github.com/spf13/cobra"
)

// Version is the CLI's own version. Wired by ldflags in releases; defaults
// to "dev" for local builds.
var Version = "dev"

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print CLI and API versions",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := struct {
				Version    string `json:"version"`
				APIVersion string `json:"api_version"`
			}{Version, api.Version}

			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "confluence %s (api %s)\n", out.Version, out.APIVersion)
			return nil
		},
	}
}
```

This imports `internal/api` for `api.Version` — Task 2 creates it. The test from Step 2 won't pass until then; that's fine, it's still failing for the same reason (missing import), and Task 2 makes it green.

- [ ] **Step 6: Commit**

```bash
git add sidecar/go.mod sidecar/go.sum sidecar/cmd/confluence/
git commit -m "confluence: add CLI skeleton (cobra) and version subcommand"
```

---

### Task 2: APIVersion constant and Error envelope types

**Files:**
- Create: `sidecar/internal/api/version.go`
- Create: `sidecar/internal/api/errors.go`
- Create: `sidecar/internal/api/errors_test.go`

- [ ] **Step 1: Write a failing test for the error envelope JSON shape**

Create `sidecar/internal/api/errors_test.go`:

```go
package api

import (
	"encoding/json"
	"testing"
)

func TestErrorResponseJSON(t *testing.T) {
	in := ErrorResponse{Error: Error{
		Code:    "scenario_invalid",
		Message: "workload.kind=replay requires reproducer.id",
		Field:   "workload.reproducer.id",
		Hint:    "set reproducer.id or change workload.kind",
	}}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const want = `{"error":{"code":"scenario_invalid","message":"workload.kind=replay requires reproducer.id","field":"workload.reproducer.id","hint":"set reproducer.id or change workload.kind"}}`
	if string(b) != want {
		t.Fatalf("got %s\nwant %s", b, want)
	}

	var rt ErrorResponse
	if err := json.Unmarshal(b, &rt); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if rt != in {
		t.Fatalf("roundtrip mismatch: got %+v want %+v", rt, in)
	}
}

func TestErrorOmitsEmptyOptionalFields(t *testing.T) {
	b, _ := json.Marshal(Error{Code: "bad", Message: "x"})
	const want = `{"code":"bad","message":"x"}`
	if string(b) != want {
		t.Fatalf("got %s\nwant %s", b, want)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/api/...`
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 3: Create the version constant**

Create `sidecar/internal/api/version.go`:

```go
// Package api defines the wire types for the confluence /v1 control API.
// Every field's JSON name is part of the contract; do not rename without
// bumping APIVersion.
package api

// Version is the wire-protocol version reported by the server and CLI.
// Frozen once shipped — breaking changes go to confluence/v2.
const Version = "confluence/v1"
```

- [ ] **Step 4: Create the error envelope**

Create `sidecar/internal/api/errors.go`:

```go
package api

// Error is a single, machine-readable error returned by the API or CLI.
// Code is from a closed, documented set per endpoint. Field and Hint are optional.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
	Hint    string `json:"hint,omitempty"`
}

// ErrorResponse is the envelope for any non-2xx HTTP response.
type ErrorResponse struct {
	Error Error `json:"error"`
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/api/...` and `go test ./cmd/confluence/...`
Expected: both PASS. The Task 1 version test now passes because `api.Version` exists.

- [ ] **Step 6: Commit**

```bash
git add sidecar/internal/api/version.go sidecar/internal/api/errors.go sidecar/internal/api/errors_test.go
git commit -m "api: APIVersion constant and error envelope types"
```

---

### Task 3: Scenario type

**Files:**
- Create: `sidecar/internal/api/scenario.go`
- Create: `sidecar/internal/api/scenario_test.go`

- [ ] **Step 1: Write a failing test for YAML→struct→JSON roundtrip**

Create `sidecar/internal/api/scenario_test.go`:

```go
package api

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

const sampleScenarioYAML = `
apiVersion: confluence/v1
kind: Scenario
metadata:
  name: soak-mixed-3x2
  description: 3 rippled + 2 goXRPL, soak workload
topology:
  rippled:
    count: 3
    image: rippleci/rippled:2.6.2
  goxrpl:
    count: 2
    image: goxrpl:latest
workload:
  kind: soak
  tx_rate: 5
  accounts: 50
  rotate_every: 1000
  mutation_rate: 0.05
chaos:
  schedule: []
observability:
  enabled: false
budget:
  duration: 10m
  stop_on:
    - first_divergence
oracles:
  - state_diff
  - consensus_liveness
  - peer_health
`

func TestScenarioYAMLRoundtrip(t *testing.T) {
	var s Scenario
	if err := yaml.Unmarshal([]byte(sampleScenarioYAML), &s); err != nil {
		t.Fatalf("yaml unmarshal: %v", err)
	}
	if s.APIVersion != "confluence/v1" {
		t.Fatalf("apiVersion: got %q", s.APIVersion)
	}
	if s.Kind != "Scenario" {
		t.Fatalf("kind: got %q", s.Kind)
	}
	if s.Metadata.Name != "soak-mixed-3x2" {
		t.Fatalf("metadata.name: got %q", s.Metadata.Name)
	}
	if s.Topology.Rippled.Count != 3 || s.Topology.Goxrpl.Count != 2 {
		t.Fatalf("topology counts: %+v", s.Topology)
	}
	if s.Workload.Kind != WorkloadSoak {
		t.Fatalf("workload.kind: got %q", s.Workload.Kind)
	}
	if s.Workload.TxRate != 5 || s.Workload.Accounts != 50 {
		t.Fatalf("workload soak fields: %+v", s.Workload)
	}
	if s.Budget.Duration != "10m" {
		t.Fatalf("budget.duration: got %q", s.Budget.Duration)
	}
	if len(s.Budget.StopOn) != 1 || s.Budget.StopOn[0] != StopOnFirstDivergence {
		t.Fatalf("budget.stop_on: %+v", s.Budget.StopOn)
	}
	if len(s.Oracles) != 3 {
		t.Fatalf("oracles: %+v", s.Oracles)
	}

	// JSON marshalling: every field uses snake_case JSON tags.
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	var asMap map[string]any
	if err := json.Unmarshal(b, &asMap); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if _, ok := asMap["api_version"]; !ok {
		t.Fatalf("expected api_version key in JSON, got %s", b)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/api/ -run TestScenario`
Expected: FAIL — `Scenario` undefined.

- [ ] **Step 3: Create the Scenario type**

Create `sidecar/internal/api/scenario.go`:

```go
package api

// Workload kinds (closed set in v1).
const (
	WorkloadSoak   = "soak"
	WorkloadFuzz   = "fuzz"
	WorkloadReplay = "replay"
	WorkloadShrink = "shrink"
	WorkloadNone   = "none"
)

// Budget stop-on conditions (closed set in v1).
const (
	StopOnFirstDivergence = "first_divergence"
	StopOnFirstCrash      = "first_crash"
	StopOnNone            = "none"
)

// Oracle names (closed set in v1).
const (
	OracleStateDiff          = "state_diff"
	OracleConsensusLiveness  = "consensus_liveness"
	OraclePeerHealth         = "peer_health"
)

// Scenario is the declarative input to `confluence run`.
// YAML tags are the public author-facing names; JSON tags are the wire shape
// (snake_case throughout).
type Scenario struct {
	APIVersion    string             `yaml:"apiVersion" json:"api_version"`
	Kind          string             `yaml:"kind" json:"kind"`
	Metadata      ScenarioMetadata   `yaml:"metadata" json:"metadata"`
	Topology      Topology           `yaml:"topology" json:"topology"`
	Workload      Workload           `yaml:"workload" json:"workload"`
	Chaos         Chaos              `yaml:"chaos,omitempty" json:"chaos,omitempty"`
	Observability Observability      `yaml:"observability,omitempty" json:"observability,omitempty"`
	Budget        Budget             `yaml:"budget" json:"budget"`
	Oracles       []string           `yaml:"oracles,omitempty" json:"oracles,omitempty"`
}

type ScenarioMetadata struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

type Topology struct {
	Rippled NodeGroup `yaml:"rippled" json:"rippled"`
	Goxrpl  NodeGroup `yaml:"goxrpl" json:"goxrpl"`
}

type NodeGroup struct {
	Count int    `yaml:"count" json:"count"`
	Image string `yaml:"image,omitempty" json:"image,omitempty"`
}

type Workload struct {
	Kind         string            `yaml:"kind" json:"kind"`
	TxRate       int               `yaml:"tx_rate,omitempty" json:"tx_rate,omitempty"`
	Accounts     int               `yaml:"accounts,omitempty" json:"accounts,omitempty"`
	RotateEvery  int               `yaml:"rotate_every,omitempty" json:"rotate_every,omitempty"`
	MutationRate float64           `yaml:"mutation_rate,omitempty" json:"mutation_rate,omitempty"`
	Reproducer   *WorkloadReproducer `yaml:"reproducer,omitempty" json:"reproducer,omitempty"`
}

type WorkloadReproducer struct {
	ID string `yaml:"id" json:"id"`
}

type Chaos struct {
	Schedule []ChaosEvent `yaml:"schedule,omitempty" json:"schedule,omitempty"`
}

// ChaosEvent mirrors today's `.chaos-schedule.json` entry shape.
// Fields are deliberately loose; the chaos runner validates internally.
type ChaosEvent struct {
	At     string         `yaml:"at,omitempty" json:"at,omitempty"`
	Kind   string         `yaml:"kind,omitempty" json:"kind,omitempty"`
	Target string         `yaml:"target,omitempty" json:"target,omitempty"`
	Params map[string]any `yaml:"params,omitempty" json:"params,omitempty"`
}

type Observability struct {
	Enabled         bool   `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	AlertWebhookURL string `yaml:"alert_webhook_url,omitempty" json:"alert_webhook_url,omitempty"`
}

type Budget struct {
	Duration string   `yaml:"duration" json:"duration"`
	StopOn   []string `yaml:"stop_on,omitempty" json:"stop_on,omitempty"`
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/api/ -run TestScenario -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add sidecar/internal/api/scenario.go sidecar/internal/api/scenario_test.go
git commit -m "api: Scenario type with YAML+JSON tags"
```

---

### Task 4: Finding type

**Files:**
- Create: `sidecar/internal/api/finding.go`
- Create: `sidecar/internal/api/finding_test.go`

- [ ] **Step 1: Write a failing test for the Finding JSON shape**

Create `sidecar/internal/api/finding_test.go`:

```go
package api

import (
	"encoding/json"
	"testing"
	"time"
)

func TestFindingJSON(t *testing.T) {
	openedAt, _ := time.Parse(time.RFC3339, "2026-05-12T14:03:21Z")
	f := Finding{
		ID:        "fnd_01HXYZ0000000000000000",
		RunID:     "run_01HXYW0000000000000000",
		EnclaveID: "xrpl-soak",
		Scenario:  "soak-mixed-3x2",
		Kind:      KindStateDivergence,
		Severity:  SeverityError,
		OpenedAt:  openedAt,
		Summary:   "goxrpl-1 disagrees with rippled-0 on AccountRoot rXYZ at ledger 1423",
		Evidence: Evidence{
			LogExcerpts: []LogExcerpt{{Node: "goxrpl-1", Lines: []string{"..."}}},
			LedgerRange: [2]uint32{1420, 1424},
			DiffKeys:    []string{"00...AccountRoot:rXYZ"},
		},
		Reproducer: &Reproducer{
			ID:           "rpr_01HXYV0000000000000000",
			ScenarioPath: ".confluence/reproducers/rpr_01HXYV0000000000000000.yaml",
			Kind:         WorkloadReplay,
		},
		SuspectedComponents: []string{"tx/payment", "ledger/state"},
	}

	b, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"id", "run_id", "enclave_id", "scenario", "kind", "severity", "opened_at", "summary", "evidence", "reproducer", "suspected_components"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("missing key %q in JSON: %s", k, b)
		}
	}
	if m["kind"] != "state_divergence" {
		t.Fatalf("kind: %v", m["kind"])
	}

	var rt Finding
	if err := json.Unmarshal(b, &rt); err != nil {
		t.Fatalf("roundtrip unmarshal: %v", err)
	}
	if rt.ID != f.ID || rt.Kind != f.Kind || rt.Evidence.LedgerRange != f.Evidence.LedgerRange {
		t.Fatalf("roundtrip mismatch")
	}
}

func TestFindingClosedAtOmitWhenZero(t *testing.T) {
	b, _ := json.Marshal(Finding{ID: "x", Kind: KindNodeCrash})
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if _, ok := m["closed_at"]; ok {
		t.Fatalf("closed_at must be omitted when zero")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/api/ -run TestFinding`
Expected: FAIL — `Finding` undefined.

- [ ] **Step 3: Create the Finding type**

Create `sidecar/internal/api/finding.go`:

```go
package api

import (
	"encoding/json"
	"time"
)

// Finding kinds (closed set in v1).
const (
	KindStateDivergence = "state_divergence"
	KindConsensusStall  = "consensus_stall"
	KindPeerDrop        = "peer_drop"
	KindNodeCrash       = "node_crash"
	KindFuzzFailure     = "fuzz_failure"
	KindChaosViolation  = "chaos_violation"
)

// Finding severities.
const (
	SeverityInfo     = "info"
	SeverityWarn     = "warn"
	SeverityError    = "error"
	SeverityCritical = "critical"
)

// Finding is a typed, durable record of something the network did wrong.
type Finding struct {
	ID                  string          `json:"id"`
	RunID               string          `json:"run_id,omitempty"`
	EnclaveID           string          `json:"enclave_id,omitempty"`
	Scenario            string          `json:"scenario,omitempty"`
	Kind                string          `json:"kind"`
	Severity            string          `json:"severity,omitempty"`
	OpenedAt            time.Time       `json:"opened_at"`
	ClosedAt            *time.Time      `json:"closed_at,omitempty"`
	Summary             string          `json:"summary,omitempty"`
	Detail              json.RawMessage `json:"detail,omitempty"`
	Evidence            Evidence        `json:"evidence,omitempty"`
	Reproducer          *Reproducer     `json:"reproducer,omitempty"`
	SuspectedComponents []string        `json:"suspected_components,omitempty"`
}

type Evidence struct {
	LogExcerpts []LogExcerpt `json:"log_excerpts,omitempty"`
	LedgerRange [2]uint32    `json:"ledger_range,omitempty"`
	DiffKeys    []string     `json:"diff_keys,omitempty"`
}

type LogExcerpt struct {
	Node  string   `json:"node"`
	Since string   `json:"since,omitempty"`
	Lines []string `json:"lines"`
}

type Reproducer struct {
	ID           string `json:"id"`
	ScenarioPath string `json:"scenario_path,omitempty"`
	Kind         string `json:"kind,omitempty"`
}
```

Note: `Evidence` is not a pointer, so `omitempty` won't drop it when all its slices are nil. That's acceptable — an empty `{}` evidence object is still readable. If we ever need to drop it, switch to `*Evidence`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/api/ -v`
Expected: PASS (all api tests, including the older ones).

- [ ] **Step 5: Commit**

```bash
git add sidecar/internal/api/finding.go sidecar/internal/api/finding_test.go
git commit -m "api: Finding type with closed kind/severity sets"
```

---

### Task 5: ID generation helpers

**Files:**
- Create: `sidecar/internal/finding/id.go`
- Create: `sidecar/internal/finding/id_test.go`

- [ ] **Step 1: Write a failing test for the ID helpers**

Create `sidecar/internal/finding/id_test.go`:

```go
package finding

import (
	"strings"
	"testing"
)

func TestIDPrefixesAndUniqueness(t *testing.T) {
	cases := []struct {
		name   string
		fn     func() string
		prefix string
	}{
		{"finding", NewFindingID, "fnd_"},
		{"run", NewRunID, "run_"},
		{"reproducer", NewReproducerID, "rpr_"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, b := tc.fn(), tc.fn()
			if !strings.HasPrefix(a, tc.prefix) {
				t.Fatalf("missing prefix %q: %q", tc.prefix, a)
			}
			if a == b {
				t.Fatalf("ids collided: %q", a)
			}
			// ULID part is 26 chars (Crockford base32).
			if len(a) != len(tc.prefix)+26 {
				t.Fatalf("unexpected length %d: %q", len(a), a)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/finding/...`
Expected: FAIL — package doesn't exist.

- [ ] **Step 3: Implement the ID helpers**

Create `sidecar/internal/finding/id.go`:

```go
// Package finding owns the durable representation of findings. M1 ships only
// the ID helpers; the store, server endpoints, and detectors land in M2.
package finding

import (
	"crypto/rand"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

var (
	entropyMu sync.Mutex
	entropy   = ulid.Monotonic(rand.Reader, 0)
)

func newID(prefix string) string {
	entropyMu.Lock()
	defer entropyMu.Unlock()
	return prefix + ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}

// NewFindingID returns a fresh "fnd_<ULID>" identifier.
func NewFindingID() string { return newID("fnd_") }

// NewRunID returns a fresh "run_<ULID>" identifier.
func NewRunID() string { return newID("run_") }

// NewReproducerID returns a fresh "rpr_<ULID>" identifier.
func NewReproducerID() string { return newID("rpr_") }
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/finding/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add sidecar/internal/finding/
git commit -m "finding: ID helpers (fnd_/run_/rpr_ prefixed ULIDs)"
```

---

### Task 6: Scenario load (file + bytes)

**Files:**
- Create: `sidecar/internal/scenario/load.go`
- Create: `sidecar/internal/scenario/load_test.go`
- Create: `sidecar/internal/scenario/testdata/soak.yaml`

- [ ] **Step 1: Write the test fixture**

Create `sidecar/internal/scenario/testdata/soak.yaml`:

```yaml
apiVersion: confluence/v1
kind: Scenario
metadata:
  name: soak-mixed-3x2
topology:
  rippled: { count: 3, image: rippleci/rippled:2.6.2 }
  goxrpl:  { count: 2, image: goxrpl:latest }
workload:
  kind: soak
  tx_rate: 5
  accounts: 50
  rotate_every: 1000
  mutation_rate: 0.05
budget:
  duration: 10m
  stop_on: [first_divergence]
oracles: [state_diff, consensus_liveness, peer_health]
```

- [ ] **Step 2: Write failing tests**

Create `sidecar/internal/scenario/load_test.go`:

```go
package scenario

import (
	"strings"
	"testing"
)

func TestLoadFile(t *testing.T) {
	s, err := Load("testdata/soak.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Metadata.Name != "soak-mixed-3x2" {
		t.Fatalf("name: %q", s.Metadata.Name)
	}
	if s.Workload.Kind != "soak" {
		t.Fatalf("workload.kind: %q", s.Workload.Kind)
	}
}

func TestParseRejectsMalformedYAML(t *testing.T) {
	_, err := Parse([]byte("not: [valid"))
	if err == nil {
		t.Fatalf("expected error on malformed YAML")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("testdata/does-not-exist.yaml")
	if err == nil {
		t.Fatalf("expected error on missing file")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Fatalf("error should mention path: %v", err)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/scenario/...`
Expected: FAIL — package doesn't exist.

- [ ] **Step 4: Implement Load and Parse**

Create `sidecar/internal/scenario/load.go`:

```go
// Package scenario loads, validates, and compiles confluence Scenario YAML.
package scenario

import (
	"fmt"
	"os"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
	"gopkg.in/yaml.v3"
)

// Load reads a Scenario YAML file from disk.
func Load(path string) (*api.Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("scenario: read %s: %w", path, err)
	}
	return Parse(data)
}

// Parse decodes Scenario YAML from bytes.
func Parse(data []byte) (*api.Scenario, error) {
	var s api.Scenario
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("scenario: parse yaml: %w", err)
	}
	return &s, nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/scenario/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add sidecar/internal/scenario/load.go sidecar/internal/scenario/load_test.go sidecar/internal/scenario/testdata/
git commit -m "scenario: Load and Parse YAML scenarios"
```

---

### Task 7: Scenario validate (semantic rules)

**Files:**
- Create: `sidecar/internal/scenario/validate.go`
- Create: `sidecar/internal/scenario/validate_test.go`

- [ ] **Step 1: Write failing table-driven tests**

Create `sidecar/internal/scenario/validate_test.go`:

```go
package scenario

import (
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
)

func validScenario() *api.Scenario {
	return &api.Scenario{
		APIVersion: "confluence/v1",
		Kind:       "Scenario",
		Metadata:   api.ScenarioMetadata{Name: "soak-mixed-3x2"},
		Topology: api.Topology{
			Rippled: api.NodeGroup{Count: 3, Image: "rippleci/rippled:2.6.2"},
			Goxrpl:  api.NodeGroup{Count: 2, Image: "goxrpl:latest"},
		},
		Workload: api.Workload{Kind: api.WorkloadSoak},
		Budget:   api.Budget{Duration: "10m", StopOn: []string{api.StopOnFirstDivergence}},
		Oracles:  []string{api.OracleStateDiff},
	}
}

func TestValidateAcceptsValidScenario(t *testing.T) {
	errs := Validate(validScenario())
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %+v", errs)
	}
}

func TestValidateRules(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(*api.Scenario)
		wantCode string
		wantField string
	}{
		{"bad apiVersion", func(s *api.Scenario) { s.APIVersion = "confluence/v0" }, "scenario_invalid", "apiVersion"},
		{"bad kind", func(s *api.Scenario) { s.Kind = "NotScenario" }, "scenario_invalid", "kind"},
		{"missing name", func(s *api.Scenario) { s.Metadata.Name = "" }, "scenario_invalid", "metadata.name"},
		{"non-kebab name", func(s *api.Scenario) { s.Metadata.Name = "Soak Mixed" }, "scenario_invalid", "metadata.name"},
		{"no nodes", func(s *api.Scenario) { s.Topology.Rippled.Count = 0; s.Topology.Goxrpl.Count = 0 }, "scenario_invalid", "topology"},
		{"negative count", func(s *api.Scenario) { s.Topology.Rippled.Count = -1 }, "scenario_invalid", "topology.rippled.count"},
		{"bad workload kind", func(s *api.Scenario) { s.Workload.Kind = "explode" }, "scenario_invalid", "workload.kind"},
		{"replay missing reproducer", func(s *api.Scenario) { s.Workload.Kind = api.WorkloadReplay }, "scenario_invalid", "workload.reproducer.id"},
		{"replay empty reproducer id", func(s *api.Scenario) { s.Workload.Kind = api.WorkloadReplay; s.Workload.Reproducer = &api.WorkloadReproducer{} }, "scenario_invalid", "workload.reproducer.id"},
		{"missing budget duration", func(s *api.Scenario) { s.Budget.Duration = "" }, "scenario_invalid", "budget.duration"},
		{"bad budget duration", func(s *api.Scenario) { s.Budget.Duration = "ten minutes" }, "scenario_invalid", "budget.duration"},
		{"bad stop_on", func(s *api.Scenario) { s.Budget.StopOn = []string{"yolo"} }, "scenario_invalid", "budget.stop_on[0]"},
		{"bad oracle", func(s *api.Scenario) { s.Oracles = []string{"nope"} }, "scenario_invalid", "oracles[0]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := validScenario()
			tc.mutate(s)
			errs := Validate(s)
			if len(errs) == 0 {
				t.Fatalf("expected error for %s, got none", tc.name)
			}
			var matched bool
			for _, e := range errs {
				if e.Code == tc.wantCode && e.Field == tc.wantField {
					matched = true
					break
				}
			}
			if !matched {
				t.Fatalf("expected code=%q field=%q, got %+v", tc.wantCode, tc.wantField, errs)
			}
		})
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/scenario/ -run TestValidate`
Expected: FAIL — `Validate` undefined.

- [ ] **Step 3: Implement Validate**

Create `sidecar/internal/scenario/validate.go`:

```go
package scenario

import (
	"fmt"
	"regexp"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
)

var kebabRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Validate runs all semantic rules over a Scenario and returns a flat list of
// api.Error values with field paths. An empty slice means the scenario is valid.
func Validate(s *api.Scenario) []api.Error {
	var errs []api.Error
	add := func(field, msg, hint string) {
		errs = append(errs, api.Error{
			Code:    "scenario_invalid",
			Message: msg,
			Field:   field,
			Hint:    hint,
		})
	}

	if s.APIVersion != api.Version {
		add("apiVersion", fmt.Sprintf("apiVersion must be %q, got %q", api.Version, s.APIVersion), "set apiVersion: confluence/v1")
	}
	if s.Kind != "Scenario" {
		add("kind", fmt.Sprintf("kind must be \"Scenario\", got %q", s.Kind), "set kind: Scenario")
	}

	if s.Metadata.Name == "" {
		add("metadata.name", "metadata.name is required", "")
	} else if !kebabRE.MatchString(s.Metadata.Name) {
		add("metadata.name", fmt.Sprintf("metadata.name must be kebab-case (got %q)", s.Metadata.Name), "use lowercase letters, digits, and single hyphens")
	}

	if s.Topology.Rippled.Count < 0 {
		add("topology.rippled.count", "topology.rippled.count must be >= 0", "")
	}
	if s.Topology.Goxrpl.Count < 0 {
		add("topology.goxrpl.count", "topology.goxrpl.count must be >= 0", "")
	}
	if s.Topology.Rippled.Count+s.Topology.Goxrpl.Count == 0 {
		add("topology", "topology must declare at least one node", "set topology.rippled.count or topology.goxrpl.count > 0")
	}

	switch s.Workload.Kind {
	case api.WorkloadSoak, api.WorkloadFuzz, api.WorkloadShrink, api.WorkloadNone:
		// ok — no extra rules at M1
	case api.WorkloadReplay:
		if s.Workload.Reproducer == nil || s.Workload.Reproducer.ID == "" {
			add("workload.reproducer.id", "workload.kind=replay requires reproducer.id", "set workload.reproducer.id or change workload.kind")
		}
	case "":
		add("workload.kind", "workload.kind is required", "one of: soak, fuzz, replay, shrink, none")
	default:
		add("workload.kind", fmt.Sprintf("unknown workload.kind %q", s.Workload.Kind), "one of: soak, fuzz, replay, shrink, none")
	}

	if s.Budget.Duration == "" {
		add("budget.duration", "budget.duration is required", "e.g. \"10m\"")
	} else if _, err := time.ParseDuration(s.Budget.Duration); err != nil {
		add("budget.duration", fmt.Sprintf("budget.duration is not a valid Go duration: %v", err), "use values like \"30s\", \"10m\", \"2h\"")
	}

	allowedStopOn := map[string]bool{
		api.StopOnFirstDivergence: true,
		api.StopOnFirstCrash:      true,
		api.StopOnNone:            true,
	}
	for i, v := range s.Budget.StopOn {
		if !allowedStopOn[v] {
			add(fmt.Sprintf("budget.stop_on[%d]", i), fmt.Sprintf("unknown stop_on value %q", v), "one of: first_divergence, first_crash, none")
		}
	}

	allowedOracle := map[string]bool{
		api.OracleStateDiff:         true,
		api.OracleConsensusLiveness: true,
		api.OraclePeerHealth:        true,
	}
	for i, v := range s.Oracles {
		if !allowedOracle[v] {
			add(fmt.Sprintf("oracles[%d]", i), fmt.Sprintf("unknown oracle %q", v), "one of: state_diff, consensus_liveness, peer_health")
		}
	}

	return errs
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/scenario/ -run TestValidate -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add sidecar/internal/scenario/validate.go sidecar/internal/scenario/validate_test.go
git commit -m "scenario: semantic validation with field-pathed api.Error output"
```

---

### Task 8: Scenario compile to Kurtosis args (golden-file test)

**Files:**
- Create: `sidecar/internal/scenario/compile.go`
- Create: `sidecar/internal/scenario/compile_test.go`
- Create: `sidecar/internal/scenario/testdata/soak-compiled.json`
- Create: `sidecar/internal/scenario/testdata/chaos.yaml`
- Create: `sidecar/internal/scenario/testdata/chaos-compiled.json`

**Reference:** the args shape today is produced by `Makefile` and consumed by `main.star`. Two shapes are locked in:

Soak (from `Makefile:soak`):
```json
{"test_suite":"soak","goxrpl_count":N,"rippled_count":M,"rippled_image":"...","goxrpl_image":"...","soak_args":{"tx_rate":T,"accounts":A,"rotate_every":R,"mutation_rate":Mr,"enable_observability":bool,"alert_webhook_url":"..."}}
```

Chaos (from `Makefile:chaos`):
```json
{"test_suite":"chaos","goxrpl_count":N,"rippled_count":M,"rippled_image":"...","goxrpl_image":"...","chaos_args":{"schedule":[...],"tx_rate":T,"accounts":A,"rotate_every":R,"mutation_rate":Mr,"enable_observability":bool,"alert_webhook_url":"..."}}
```

- [ ] **Step 1: Write the soak golden file**

Create `sidecar/internal/scenario/testdata/soak-compiled.json`:

```json
{"test_suite":"soak","goxrpl_count":2,"rippled_count":3,"rippled_image":"rippleci/rippled:2.6.2","goxrpl_image":"goxrpl:latest","soak_args":{"tx_rate":5,"accounts":50,"rotate_every":1000,"mutation_rate":0.05,"enable_observability":false,"alert_webhook_url":""}}
```

- [ ] **Step 2: Write the chaos input + golden file**

Create `sidecar/internal/scenario/testdata/chaos.yaml`:

```yaml
apiVersion: confluence/v1
kind: Scenario
metadata:
  name: chaos-mixed-3x2
topology:
  rippled: { count: 3, image: rippleci/rippled:2.6.2 }
  goxrpl:  { count: 2, image: goxrpl:latest }
workload:
  kind: fuzz
  tx_rate: 5
  accounts: 50
  rotate_every: 1000
  mutation_rate: 0.05
chaos:
  schedule:
    - { at: "30s", kind: latency, target: goxrpl-0, params: { delay_ms: 200 } }
observability:
  enabled: true
  alert_webhook_url: "https://example.invalid/hook"
budget:
  duration: 10m
oracles: [state_diff]
```

Create `sidecar/internal/scenario/testdata/chaos-compiled.json`:

```json
{"test_suite":"chaos","goxrpl_count":2,"rippled_count":3,"rippled_image":"rippleci/rippled:2.6.2","goxrpl_image":"goxrpl:latest","chaos_args":{"schedule":[{"at":"30s","kind":"latency","target":"goxrpl-0","params":{"delay_ms":200}}],"tx_rate":5,"accounts":50,"rotate_every":1000,"mutation_rate":0.05,"enable_observability":true,"alert_webhook_url":"https://example.invalid/hook"}}
```

- [ ] **Step 3: Write failing tests**

Create `sidecar/internal/scenario/compile_test.go`:

```go
package scenario

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestCompileSoakGolden(t *testing.T) {
	s, err := Load("testdata/soak.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	got, err := Compile(s)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	assertJSONEqualToFile(t, got, "testdata/soak-compiled.json")
}

func TestCompileChaosGolden(t *testing.T) {
	s, err := Load("testdata/chaos.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	got, err := Compile(s)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	assertJSONEqualToFile(t, got, "testdata/chaos-compiled.json")
}

func TestCompileRejectsInvalidScenario(t *testing.T) {
	s, err := Load("testdata/soak.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	s.Metadata.Name = "" // make it invalid
	if _, err := Compile(s); err == nil {
		t.Fatalf("expected compile to reject invalid scenarios")
	}
}

func TestCompileRejectsReplay(t *testing.T) {
	// Replay scenarios are not compiled directly; they flow through
	// `confluence replay`, which composes its own kurtosis input.
	s, err := Load("testdata/soak.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	s.Workload.Kind = "replay"
	s.Workload.Reproducer = nil // intentionally invalid for replay, to exercise both paths
	if _, err := Compile(s); err == nil || !strings.Contains(err.Error(), "replay") {
		t.Fatalf("expected replay rejection, got err=%v", err)
	}
}

func assertJSONEqualToFile(t *testing.T, got []byte, path string) {
	t.Helper()
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	if !jsonEqual(t, got, want) {
		t.Fatalf("compile mismatch\n got: %s\nwant: %s", got, bytes.TrimSpace(want))
	}
}

func jsonEqual(t *testing.T, a, b []byte) bool {
	t.Helper()
	var ax, bx any
	if err := json.Unmarshal(a, &ax); err != nil {
		t.Fatalf("got is not JSON: %v (%s)", err, a)
	}
	if err := json.Unmarshal(b, &bx); err != nil {
		t.Fatalf("want is not JSON: %v", err)
	}
	ja, _ := json.Marshal(ax)
	jb, _ := json.Marshal(bx)
	return bytes.Equal(ja, jb)
}
```

Note: `TestCompileRejectsReplay` sets `Reproducer = nil` so `Validate` reports the replay-without-reproducer error first; `Compile` returns the validation error. If Validate passes a replay scenario in the future (it currently won't), the explicit `replay` check in `Compile` is the second line of defence — the test asserts the error message contains `replay` either way.

- [ ] **Step 4: Run the tests to verify they fail**

Run: `go test ./internal/scenario/ -run TestCompile`
Expected: FAIL — `Compile` undefined.

- [ ] **Step 5: Implement Compile**

Create `sidecar/internal/scenario/compile.go`:

```go
package scenario

import (
	"encoding/json"
	"fmt"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
)

// Compile turns a validated Scenario into the JSON args object that
// `kurtosis run` passes to main.star. This is the single source of truth for
// the args shape — Makefile and other callers must not hand-roll it.
//
// Replay scenarios are not compilable here: they flow through
// `confluence replay`, which composes its own kurtosis input from the
// reproducer's source scenario.
func Compile(s *api.Scenario) ([]byte, error) {
	if errs := Validate(s); len(errs) > 0 {
		return nil, fmt.Errorf("scenario invalid: %d error(s); first: %s (%s)", len(errs), errs[0].Message, errs[0].Field)
	}
	if s.Workload.Kind == api.WorkloadReplay {
		return nil, fmt.Errorf("scenario: replay workloads are not compiled directly; use confluence replay")
	}

	out := map[string]any{
		"test_suite":    workloadToTestSuite(s.Workload.Kind),
		"goxrpl_count":  s.Topology.Goxrpl.Count,
		"rippled_count": s.Topology.Rippled.Count,
		"rippled_image": s.Topology.Rippled.Image,
		"goxrpl_image":  s.Topology.Goxrpl.Image,
	}

	workArgs := map[string]any{
		"tx_rate":              s.Workload.TxRate,
		"accounts":             s.Workload.Accounts,
		"rotate_every":         s.Workload.RotateEvery,
		"mutation_rate":        s.Workload.MutationRate,
		"enable_observability": s.Observability.Enabled,
		"alert_webhook_url":    s.Observability.AlertWebhookURL,
	}

	switch s.Workload.Kind {
	case api.WorkloadSoak:
		out["soak_args"] = workArgs
	case api.WorkloadFuzz:
		// Chaos uses fuzz workload + non-empty schedule. Empty schedule = pure fuzz.
		if len(s.Chaos.Schedule) > 0 {
			out["test_suite"] = "chaos"
			chaosArgs := map[string]any{"schedule": s.Chaos.Schedule}
			for k, v := range workArgs {
				chaosArgs[k] = v
			}
			out["chaos_args"] = chaosArgs
		} else {
			out["fuzz_args"] = workArgs
		}
	case api.WorkloadShrink:
		out["shrink_args"] = workArgs
	case api.WorkloadNone:
		// no workload args
	}

	return json.Marshal(out)
}

// workloadToTestSuite maps a scenario workload kind to the existing
// main.star test_suite value. The chaos override is applied in Compile after
// the schedule check.
func workloadToTestSuite(kind string) string {
	switch kind {
	case api.WorkloadSoak:
		return "soak"
	case api.WorkloadFuzz:
		return "fuzz"
	case api.WorkloadShrink:
		return "shrink"
	case api.WorkloadNone:
		return "none"
	}
	return kind
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/scenario/ -v`
Expected: PASS (including the new compile tests and the older load/validate tests).

- [ ] **Step 7: Commit**

```bash
git add sidecar/internal/scenario/compile.go sidecar/internal/scenario/compile_test.go sidecar/internal/scenario/testdata/
git commit -m "scenario: compile to kurtosis main.star args (golden-file test)"
```

---

### Task 9: `confluence scenario validate` subcommand

**Files:**
- Create: `sidecar/cmd/confluence/scenario.go`
- Create: `sidecar/cmd/confluence/scenario_test.go`
- Modify: `sidecar/cmd/confluence/main.go` (wire the new subcommand)

- [ ] **Step 1: Write the failing tests**

Create `sidecar/cmd/confluence/scenario_test.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runCmd(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	outBuf, errBuf := &bytes.Buffer{}, &bytes.Buffer{}
	root := newRootCmd()
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	root.SetArgs(args)
	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

func writeTempYAML(t *testing.T, body string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "scenario-*.yaml")
	if err != nil {
		t.Fatalf("temp: %v", err)
	}
	if _, err := f.WriteString(body); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = f.Close()
	return f.Name()
}

func TestScenarioValidateValid(t *testing.T) {
	path := filepath.Join("..", "..", "internal", "scenario", "testdata", "soak.yaml")
	stdout, _, err := runCmd(t, "scenario", "validate", "--json", path)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got struct {
		OK     bool             `json:"ok"`
		Errors []map[string]any `json:"errors"`
	}
	if jerr := json.Unmarshal([]byte(stdout), &got); jerr != nil {
		t.Fatalf("not JSON: %v (out=%q)", jerr, stdout)
	}
	if !got.OK || len(got.Errors) != 0 {
		t.Fatalf("expected ok with no errors, got %+v", got)
	}
}

func TestScenarioValidateInvalid(t *testing.T) {
	path := writeTempYAML(t, "kind: NotScenario\n")
	stdout, _, err := runCmd(t, "scenario", "validate", "--json", path)
	if err == nil {
		t.Fatalf("expected non-nil error on invalid scenario")
	}
	if !strings.Contains(stdout, `"ok":false`) {
		t.Fatalf("expected ok:false in output, got %q", stdout)
	}
	var got struct {
		OK     bool             `json:"ok"`
		Errors []map[string]any `json:"errors"`
	}
	if jerr := json.Unmarshal([]byte(stdout), &got); jerr != nil {
		t.Fatalf("not JSON: %v", jerr)
	}
	if got.OK || len(got.Errors) == 0 {
		t.Fatalf("expected errors, got %+v", got)
	}
}

func TestScenarioValidateHumanOutput(t *testing.T) {
	path := writeTempYAML(t, "kind: NotScenario\n")
	stdout, _, err := runCmd(t, "scenario", "validate", path)
	if err == nil {
		t.Fatalf("expected non-nil error")
	}
	if !strings.Contains(stdout, "kind") {
		t.Fatalf("expected human output to reference the bad field, got %q", stdout)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./cmd/confluence/ -run TestScenario`
Expected: FAIL — `scenario` subcommand undefined.

- [ ] **Step 3: Implement the subcommand**

Create `sidecar/cmd/confluence/scenario.go`:

```go
package main

import (
	"encoding/json"
	"fmt"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/scenario"
	"github.com/spf13/cobra"
)

func newScenarioCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scenario",
		Short: "Manage Scenario files",
	}
	cmd.AddCommand(newScenarioValidateCmd())
	return cmd
}

func newScenarioValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate PATH",
		Short: "Validate a Scenario YAML file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := scenario.Load(args[0])
			if err != nil {
				return outputValidation(cmd, false, []api.Error{{
					Code:    "scenario_unreadable",
					Message: err.Error(),
				}})
			}
			errs := scenario.Validate(s)
			ok := len(errs) == 0
			return outputValidation(cmd, ok, errs)
		},
	}
}

func outputValidation(cmd *cobra.Command, ok bool, errs []api.Error) error {
	asJSON, _ := cmd.Flags().GetBool("json")
	if errs == nil {
		errs = []api.Error{}
	}

	if asJSON {
		payload := struct {
			OK     bool        `json:"ok"`
			Errors []api.Error `json:"errors"`
		}{ok, errs}
		if jerr := json.NewEncoder(cmd.OutOrStdout()).Encode(payload); jerr != nil {
			return jerr
		}
	} else if ok {
		fmt.Fprintln(cmd.OutOrStdout(), "ok")
	} else {
		for _, e := range errs {
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", e.Field, e.Message)
		}
	}

	if !ok {
		// Returning an error makes cobra propagate a non-zero exit; we suppress
		// printing it (SilenceErrors on root) so JSON consumers see only the JSON
		// payload on stdout.
		return fmt.Errorf("scenario invalid")
	}
	return nil
}
```

- [ ] **Step 4: Wire the new subcommand into the root**

Modify `sidecar/cmd/confluence/main.go` — find `root.AddCommand(newVersionCmd())` and add the scenario command next to it:

```go
	root.AddCommand(newVersionCmd())
	root.AddCommand(newScenarioCmd())
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./cmd/confluence/ -v`
Expected: PASS (version + scenario tests).

- [ ] **Step 6: Commit**

```bash
git add sidecar/cmd/confluence/scenario.go sidecar/cmd/confluence/scenario_test.go sidecar/cmd/confluence/main.go
git commit -m "confluence: scenario validate subcommand (human + --json output)"
```

---

### Task 10: Built-in scenario file + smoke test

**Files:**
- Create: `scenarios/soak-mixed-3x2.yaml` (at repo root, not under `sidecar/`)
- Create: `sidecar/internal/scenario/builtin_test.go`

- [ ] **Step 1: Create the built-in scenario**

From the repo root, create `scenarios/soak-mixed-3x2.yaml`:

```yaml
apiVersion: confluence/v1
kind: Scenario
metadata:
  name: soak-mixed-3x2
  description: 3 rippled + 2 goXRPL, soak workload, 10 min budget
topology:
  rippled:
    count: 3
    image: rippleci/rippled:2.6.2
  goxrpl:
    count: 2
    image: goxrpl:latest
workload:
  kind: soak
  tx_rate: 5
  accounts: 50
  rotate_every: 1000
  mutation_rate: 0.05
observability:
  enabled: false
budget:
  duration: 10m
  stop_on:
    - first_divergence
oracles:
  - state_diff
  - consensus_liveness
  - peer_health
```

- [ ] **Step 2: Write a smoke test that walks every YAML file under `scenarios/`**

Create `sidecar/internal/scenario/builtin_test.go`:

```go
package scenario

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuiltinScenariosLoadAndCompile guards every YAML under ../../../scenarios/
// against drift: each file must load, validate, and (unless it's a replay
// template) compile cleanly. New built-ins get this test for free.
func TestBuiltinScenariosLoadAndCompile(t *testing.T) {
	root := filepath.Join("..", "..", "..", "scenarios")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Skipf("no scenarios dir at %s: %v", root, err)
		return
	}
	var found int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		found++
		t.Run(e.Name(), func(t *testing.T) {
			path := filepath.Join(root, e.Name())
			s, err := Load(path)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if errs := Validate(s); len(errs) != 0 {
				t.Fatalf("validate: %+v", errs)
			}
			if s.Workload.Kind == "replay" {
				return // replay templates don't compile
			}
			if _, err := Compile(s); err != nil {
				t.Fatalf("compile: %v", err)
			}
		})
	}
	if found == 0 {
		t.Fatalf("expected at least one built-in scenario under %s", root)
	}
}
```

- [ ] **Step 3: Run the test**

Run from `sidecar/`: `go test ./internal/scenario/ -run TestBuiltin -v`
Expected: PASS — `soak-mixed-3x2.yaml` loads, validates, and compiles.

- [ ] **Step 4: Full test sweep**

Run from `sidecar/`: `go test ./...`
Expected: PASS across all packages.

- [ ] **Step 5: Commit**

```bash
git add scenarios/soak-mixed-3x2.yaml sidecar/internal/scenario/builtin_test.go
git commit -m "scenarios: ship soak-mixed-3x2 built-in scenario (guarded by smoke test)"
```

---

## Final verification

- [ ] **Run the full test suite**

From `sidecar/`: `go test ./...`
Expected: all packages PASS.

- [ ] **Build the CLI**

From `sidecar/`: `go build -o /tmp/confluence ./cmd/confluence && /tmp/confluence version --json`
Expected: prints something like `{"version":"dev","api_version":"confluence/v1"}`.

- [ ] **Drive the CLI against the built-in scenario**

From repo root: `/tmp/confluence scenario validate scenarios/soak-mixed-3x2.yaml --json`
Expected: `{"ok":true,"errors":[]}`, exit 0.

From repo root: `/tmp/confluence scenario validate /dev/null --json`
Expected: `{"ok":false,"errors":[...]}` with non-zero exit (`echo $?` → 1).

---

## What this milestone leaves for M2

M2 ("Control service in-enclave") needs:
- `sidecar/internal/finding/store.go` — JSON-on-disk finding store; the M1 ID helpers feed it.
- `sidecar/internal/server/` — HTTP handlers for `/v1/scenarios`, `/v1/findings`, `/v1/healthz` (full set listed in the spec).
- `sidecar/cmd/confluence-control/` — server binary; loads scenarios via `scenario.Load`, validates via `scenario.Validate`, advertises `api.Version`.
- Wire the existing `internal/oracle/` and `internal/fuzz/` to publish `api.Finding` values into the store.

None of the M2 work modifies anything M1 ships — `internal/api/` and `internal/scenario/` are dependency-free, and the M1 CLI gains subcommands without restructuring.
