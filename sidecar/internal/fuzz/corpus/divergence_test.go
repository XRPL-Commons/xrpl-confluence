package corpus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecorder_WritesDivergenceJSON(t *testing.T) {
	dir := t.TempDir()
	rec := NewRecorder(dir, 42)

	first, err := rec.RecordDivergence(&Divergence{
		Seed:        42,
		Kind:        "state_hash",
		Description: "node A != node B",
		Details:     map[string]any{"seq": 1234},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !first {
		t.Errorf("want isFirstSeen=true on first record, got false")
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
		if _, err := rec.RecordDivergence(&Divergence{Seed: 7, Kind: "tx_result"}); err != nil {
			t.Fatal(err)
		}
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "divergences"))
	if len(entries) != 10 {
		t.Fatalf("got %d files, want 10 (names must be unique)", len(entries))
	}
}

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

	// Third with a different tx_type — different signature, isFirstSeen=true again.
	third, err := rec.RecordDivergence(&Divergence{Kind: "tx_result", Details: map[string]any{"tx_type": "OfferCreate"}})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if !third {
		t.Errorf("want isFirstSeen=true on different-signature record")
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

	// Per-divergence files still get written.
	entries, _ := os.ReadDir(filepath.Join(dir, "divergences"))
	if len(entries) != 3 {
		t.Errorf("want 3 raw divergence files, got %d", len(entries))
	}
}
