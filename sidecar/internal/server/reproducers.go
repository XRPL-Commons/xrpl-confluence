package server

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/finding"
)

// ReproducerEmitter writes a replay scenario YAML for a trigger finding.
type ReproducerEmitter struct {
	dir   string
	store *finding.Store
}

// NewReproducerEmitter creates a ReproducerEmitter that writes to dir and
// patches findings in store after emission.
func NewReproducerEmitter(dir string, store *finding.Store) *ReproducerEmitter {
	return &ReproducerEmitter{dir: dir, store: store}
}

// Emit clones sc, rewrites it as a replay scenario, writes it to disk, and
// patches the trigger finding in the store with the new reproducer.
func (e *ReproducerEmitter) Emit(sc api.Scenario, trigger *api.Finding) (*api.Reproducer, error) {
	id := finding.NewReproducerID()

	clone := sc
	clone.Workload.Kind = api.WorkloadReplay
	clone.Workload.Reproducer = &api.WorkloadReproducer{ID: id}

	data, err := yaml.Marshal(clone)
	if err != nil {
		return nil, fmt.Errorf("marshal scenario: %w", err)
	}

	if err := os.MkdirAll(e.dir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir reproducers dir: %w", err)
	}

	path := filepath.Join(e.dir, id+".yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, fmt.Errorf("write reproducer: %w", err)
	}

	rep := &api.Reproducer{
		ID:           id,
		ScenarioPath: ".confluence/reproducers/" + id + ".yaml",
		Kind:         api.WorkloadReplay,
	}

	if !e.store.UpdateReproducer(trigger.ID, rep) {
		// Finding may have been evicted — not fatal, just log.
		_ = id // already written to disk
	}

	return rep, nil
}
