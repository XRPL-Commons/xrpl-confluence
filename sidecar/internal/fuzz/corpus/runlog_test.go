package corpus

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunLog_AppendAndRead(t *testing.T) {
	dir := t.TempDir()
	w, err := NewRunLog(dir, 0xdeadbeef)
	if err != nil {
		t.Fatal(err)
	}

	entries := []RunLogEntry{
		{Step: 0, TxType: "Payment", Fields: map[string]any{"Account": "rA", "Amount": "1000"}, Secret: "s1", Result: "tesSUCCESS"},
		{Step: 1, TxType: "TrustSet", Fields: map[string]any{"Account": "rB"}, Secret: "s2", Result: "tesSUCCESS"},
	}
	for _, e := range entries {
		if err := w.Append(&e); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	// File should exist at deterministic path.
	path := filepath.Join(dir, "runs", "deadbeef.ndjson")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("run log file missing: %v", err)
	}

	// Reader round-trip.
	got, err := ReadRunLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(entries) {
		t.Fatalf("got %d entries, want %d", len(got), len(entries))
	}
	for i, e := range entries {
		if got[i].Step != e.Step || got[i].TxType != e.TxType || got[i].Secret != e.Secret {
			t.Fatalf("entry %d mismatch: %+v vs %+v", i, got[i], e)
		}
	}
}

func TestRunLog_IdempotentSeedPath(t *testing.T) {
	dir := t.TempDir()
	w1, _ := NewRunLog(dir, 42)
	w1.Close()
	w2, _ := NewRunLog(dir, 42)
	w2.Close()
	// Both should produce the same path.
	entries, _ := os.ReadDir(filepath.Join(dir, "runs"))
	if len(entries) != 1 {
		t.Fatalf("want 1 file, got %d", len(entries))
	}
}
