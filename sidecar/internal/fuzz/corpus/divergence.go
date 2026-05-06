package corpus

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// Divergence is the canonical representation of "nodes disagreed about
// something". It is written to disk verbatim so a reproducer can be assembled
// from (Seed, Details).
type Divergence struct {
	Seed        uint64         `json:"seed"`
	Kind        string         `json:"kind"` // "state_hash" | "tx_result" | "metadata" | "invariant" | "crash"
	Description string         `json:"description,omitempty"`
	Details     map[string]any `json:"details,omitempty"`
	RecordedAt  time.Time      `json:"recorded_at"`
}

// Recorder owns the on-disk corpus for a single fuzz run.
type Recorder struct {
	baseDir string
	seed    uint64
	counter atomic.Uint64
}

// NewRecorder creates a Recorder writing under baseDir/divergences/.
func NewRecorder(baseDir string, seed uint64) *Recorder {
	return &Recorder{baseDir: baseDir, seed: seed}
}

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
		// Surface the error to the caller alongside firstSeen=false.
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
