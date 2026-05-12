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
