// Package manifest writes a single run-manifest.json next to a fuzz corpus
// so that a divergence saved days later can be re-driven with identical
// config (seed, account count, tier weights, mutation rate, schedule, etc).
package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Manifest captures every knob that influences a fuzz run. Anything not
// reproducible from the run-manifest plus FUZZ_SEED should be added here.
type Manifest struct {
	StartedAt   time.Time      `json:"started_at"`
	Mode        string         `json:"mode"`
	Seed        uint64         `json:"seed"`
	Accounts    int            `json:"accounts"`
	TxCount     int            `json:"tx_count,omitempty"`
	TxRate      float64        `json:"tx_rate,omitempty"`
	Rotate      int64          `json:"rotate_every,omitempty"`
	Mutation    float64        `json:"mutation_rate,omitempty"`
	LocalSign   bool           `json:"local_sign"`
	Nodes       []string       `json:"node_urls"`
	SubmitURL   string         `json:"submit_url"`
	CorpusDir   string         `json:"corpus_dir"`
	BatchClose  string         `json:"batch_close,omitempty"`
	TierWeights map[string]int `json:"tier_weights,omitempty"`
	Schedule    string         `json:"chaos_schedule,omitempty"`
	LedgerStart int            `json:"ledger_start,omitempty"`
	LedgerEnd   int            `json:"ledger_end,omitempty"`
	Image       string         `json:"image,omitempty"`
	GitSHA      string         `json:"git_sha,omitempty"`
}

// Write serialises m to <corpusDir>/run-manifest.json. StartedAt is set to
// time.Now() if zero. Parent directories are created.
func Write(corpusDir string, m Manifest) error {
	if m.StartedAt.IsZero() {
		m.StartedAt = time.Now().UTC()
	}
	if err := os.MkdirAll(corpusDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", corpusDir, err)
	}
	path := filepath.Join(corpusDir, "run-manifest.json")
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}
