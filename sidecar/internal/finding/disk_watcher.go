package finding

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
)

// DiskWatcher polls <dir>/divergences/*.json and ingests new files into a Store.
type DiskWatcher struct {
	dir      string
	interval time.Duration
	store    *Store
	seen     map[string]bool
	cancel   context.CancelFunc
}

func NewDiskWatcher(dir string, store *Store, interval time.Duration) *DiskWatcher {
	return &DiskWatcher{
		dir:      dir,
		interval: interval,
		store:    store,
		seen:     make(map[string]bool),
	}
}

// Start spawns the polling goroutine. It stops when ctx is cancelled or Stop
// is called.
func (w *DiskWatcher) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	go w.run(ctx)
}

// Stop cancels the watcher's internal context, causing the goroutine to exit.
func (w *DiskWatcher) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
}

func (w *DiskWatcher) run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.scan()
		}
	}
}

func (w *DiskWatcher) scan() {
	divDir := filepath.Join(w.dir, "divergences")
	entries, err := os.ReadDir(divDir)
	if err != nil {
		// Dir not yet created — silent no-op.
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if filepath.Ext(name) != ".json" {
			continue
		}
		if w.seen[name] {
			continue
		}
		w.seen[name] = true
		w.ingest(filepath.Join(divDir, name))
	}
}

func (w *DiskWatcher) ingest(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("disk_watcher: read %s: %v", path, err)
		return
	}
	var d corpus.Divergence
	if err := json.Unmarshal(data, &d); err != nil {
		log.Printf("disk_watcher: parse %s: %v", path, err)
		return
	}
	f, err := MapDivergence(d)
	if err != nil {
		log.Printf("disk_watcher: map %s: %v", path, err)
		return
	}
	w.store.Add(f)
}
