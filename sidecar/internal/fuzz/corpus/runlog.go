package corpus

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// RunLogEntry is one row in an ndjson run log — captures everything needed
// to re-submit the same tx against a fresh topology.
type RunLogEntry struct {
	Step   int            `json:"step"`
	TxType string         `json:"tx_type"`
	Fields map[string]any `json:"fields"`
	Secret string         `json:"secret"`
	Result string         `json:"result,omitempty"` // EngineResult at original submit
	TxHash string         `json:"tx_hash,omitempty"`
}

// RunLog appends RunLogEntry rows to <baseDir>/runs/<seed>.ndjson. Safe for
// concurrent Append calls from a single process.
type RunLog struct {
	mu   sync.Mutex
	f    *os.File
	bw   *bufio.Writer
	path string
	seed uint64
}

// NewRunLog opens (creating/truncating) <baseDir>/runs/<seed>.ndjson.
// Seed is formatted as lowercase hex for filename stability.
func NewRunLog(baseDir string, seed uint64) (*RunLog, error) {
	dir := filepath.Join(baseDir, "runs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, fmt.Sprintf("%x.ndjson", seed))
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create %s: %w", path, err)
	}
	return &RunLog{f: f, bw: bufio.NewWriter(f), path: path, seed: seed}, nil
}

// Path returns the on-disk path of this log.
func (r *RunLog) Path() string { return r.path }

// Append writes one entry. Safe for concurrent use.
func (r *RunLog) Append(e *RunLogEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal run-log entry: %w", err)
	}
	if _, err := r.bw.Write(line); err != nil {
		return err
	}
	if err := r.bw.WriteByte('\n'); err != nil {
		return err
	}
	// Flush per-entry so an abrupt shutdown still leaves a valid ndjson.
	return r.bw.Flush()
}

// Close flushes and closes the underlying file.
func (r *RunLog) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	if err := r.bw.Flush(); err != nil {
		return err
	}
	err := r.f.Close()
	r.f = nil
	return err
}

// ReadRunLog reads an ndjson file back into a slice of entries.
func ReadRunLog(path string) ([]RunLogEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []RunLogEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<22) // larger-than-default for big Fields maps
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e RunLogEntry
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("parse line: %w", err)
		}
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
