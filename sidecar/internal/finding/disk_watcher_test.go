package finding

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
)

func writeDivergence(t *testing.T, dir, name string, d corpus.Divergence) {
	t.Helper()
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal divergence: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func TestDiskWatcher(t *testing.T) {
	baseDir := t.TempDir()
	divDir := filepath.Join(baseDir, "divergences")
	if err := os.MkdirAll(divDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	store := NewStore()
	watcher := NewDiskWatcher(baseDir, store, 50*time.Millisecond)
	watcher.Start(t.Context())

	d1 := corpus.Divergence{
		Seed:        1,
		Kind:        "state_hash",
		Description: "state diverged",
		RecordedAt:  time.Now().UTC(),
	}
	writeDivergence(t, divDir, "first.json", d1)

	time.Sleep(150 * time.Millisecond)
	if store.Len() != 1 {
		t.Fatalf("expected 1 finding, got %d", store.Len())
	}
	list := store.List(ListOpts{})
	if list[0].Summary != d1.Description {
		t.Errorf("summary: got %q, want %q", list[0].Summary, d1.Description)
	}

	d2 := corpus.Divergence{
		Seed:        2,
		Kind:        "crash",
		Description: "node crashed",
		RecordedAt:  time.Now().UTC(),
	}
	writeDivergence(t, divDir, "second.json", d2)

	time.Sleep(150 * time.Millisecond)
	if store.Len() != 2 {
		t.Fatalf("expected 2 findings, got %d", store.Len())
	}

	// Malformed JSON — should be silently skipped.
	if err := os.WriteFile(filepath.Join(divDir, "bad.json"), []byte("not json{{{"), 0o644); err != nil {
		t.Fatalf("write bad file: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	if store.Len() != 2 {
		t.Fatalf("expected still 2 findings after bad file, got %d", store.Len())
	}

	// Stop should terminate promptly.
	done := make(chan struct{})
	go func() {
		watcher.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("watcher did not stop within 200ms")
	}
}

func TestDiskWatcher_NonExistentDir(t *testing.T) {
	baseDir := t.TempDir()
	store := NewStore()
	watcher := NewDiskWatcher(baseDir, store, 50*time.Millisecond)
	watcher.Start(t.Context())
	time.Sleep(100 * time.Millisecond)
	if store.Len() != 0 {
		t.Fatalf("expected 0, got %d", store.Len())
	}
	watcher.Stop()
}
