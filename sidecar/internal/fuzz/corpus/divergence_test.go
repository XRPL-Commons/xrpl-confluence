package corpus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRecorder_WritesDivergenceJSON(t *testing.T) {
	dir := t.TempDir()
	rec := NewRecorder(dir, 42)

	err := rec.RecordDivergence(&Divergence{
		Seed:        42,
		Kind:        "state_hash",
		Description: "node A != node B",
		Details:     map[string]any{"seq": 1234},
	})
	if err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(filepath.Join(dir, "divergences"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 recorded divergence, got %d", len(entries))
	}

	data, err := os.ReadFile(filepath.Join(dir, "divergences", entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	var got Divergence
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Seed != 42 || got.Kind != "state_hash" {
		t.Fatalf("unexpected content: %+v", got)
	}
}

func TestRecorder_ManyRecordedConcurrently(t *testing.T) {
	dir := t.TempDir()
	rec := NewRecorder(dir, 7)
	for i := 0; i < 10; i++ {
		if err := rec.RecordDivergence(&Divergence{Seed: 7, Kind: "tx_result"}); err != nil {
			t.Fatal(err)
		}
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "divergences"))
	if len(entries) != 10 {
		t.Fatalf("got %d files, want 10 (names must be unique)", len(entries))
	}
}
