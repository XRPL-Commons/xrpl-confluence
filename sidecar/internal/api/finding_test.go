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
