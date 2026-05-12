package server_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/finding"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/server"
)

func TestReproducerEmitter_Emit(t *testing.T) {
	dir := t.TempDir()
	store := finding.NewStore()

	trigger := api.Finding{
		ID:       finding.NewFindingID(),
		Kind:     api.KindStateDivergence,
		OpenedAt: time.Now(),
		Summary:  "state diverged",
	}
	store.Add(trigger)

	sc := api.Scenario{
		APIVersion: "confluence/v1",
		Kind:       "Scenario",
		Metadata:   api.ScenarioMetadata{Name: "my-scenario"},
		Topology: api.Topology{
			Rippled: api.NodeGroup{Count: 1},
			Goxrpl:  api.NodeGroup{Count: 1},
		},
		Workload: api.Workload{Kind: api.WorkloadSoak},
		Budget:   api.Budget{Duration: "10m"},
	}

	emitter := server.NewReproducerEmitter(dir, store)
	rep, err := emitter.Emit(sc, &trigger)
	if err != nil {
		t.Fatalf("Emit returned error: %v", err)
	}
	if rep == nil {
		t.Fatal("Emit returned nil reproducer")
	}
	if rep.ID == "" {
		t.Fatal("reproducer ID is empty")
	}
	if rep.Kind != api.WorkloadReplay {
		t.Fatalf("expected kind %q, got %q", api.WorkloadReplay, rep.Kind)
	}

	// File must exist on disk.
	path := filepath.Join(dir, rep.ID+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reproducer file not found: %v", err)
	}

	// Re-parse and verify it is a valid replay scenario with the right reproducer ID.
	var parsed api.Scenario
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("re-parse YAML: %v", err)
	}
	if parsed.Workload.Kind != api.WorkloadReplay {
		t.Errorf("parsed workload.kind=%q, want %q", parsed.Workload.Kind, api.WorkloadReplay)
	}
	if parsed.Workload.Reproducer == nil {
		t.Fatal("parsed workload.reproducer is nil")
	}
	if parsed.Workload.Reproducer.ID != rep.ID {
		t.Errorf("parsed reproducer.id=%q, want %q", parsed.Workload.Reproducer.ID, rep.ID)
	}

	// The finding in the store must now have its Reproducer field set.
	updated, ok := store.GetByID(trigger.ID)
	if !ok {
		t.Fatal("trigger finding not found in store")
	}
	if updated.Reproducer == nil {
		t.Fatal("finding.Reproducer is nil after Emit")
	}
	if updated.Reproducer.ID != rep.ID {
		t.Errorf("finding.Reproducer.ID=%q, want %q", updated.Reproducer.ID, rep.ID)
	}
}
