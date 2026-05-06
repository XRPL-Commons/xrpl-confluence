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
		Mode:        "chaos",
		Seed:        42,
		Accounts:    50,
		TxRate:      5,
		Mutation:    0.05,
		Rotate:      1000,
		LocalSign:   false,
		Nodes:       []string{"http://rippled-0:5005", "http://rippled-1:5005"},
		SubmitURL:   "http://rippled-0:5005",
		TierWeights: map[string]int{"rich": 1, "at_reserve": 0},
		Schedule:    `[{"step":50,"type":"restart","container":"rippled-1"}]`,
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
	if back.TierWeights["rich"] != 1 {
		t.Errorf("tier weights lost: %+v", back.TierWeights)
	}
}

func TestWrite_MkdirParents(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "deep", "path")
	if err := Write(dir, Manifest{Mode: "soak", Seed: 1}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "run-manifest.json")); err != nil {
		t.Errorf("manifest not created: %v", err)
	}
}

func TestWrite_PreservesExplicitStartedAt(t *testing.T) {
	dir := t.TempDir()
	m := Manifest{Mode: "fuzz", Seed: 1}
	// Default StartedAt is zero → Write should populate it.
	if err := Write(dir, m); err != nil {
		t.Fatalf("write: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "run-manifest.json"))
	var back Manifest
	_ = json.Unmarshal(data, &back)
	if back.StartedAt.IsZero() {
		t.Errorf("StartedAt should be populated by Write when zero")
	}
}
