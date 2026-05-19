package finding

import (
	"sync"
	"testing"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
)

type fakeProgress struct {
	mu     sync.Mutex
	inputs []ConsensusProgressInput
}

func (f *fakeProgress) ConsensusProgressSnapshot() []ConsensusProgressInput {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]ConsensusProgressInput, len(f.inputs))
	copy(out, f.inputs)
	return out
}

func (f *fakeProgress) set(in []ConsensusProgressInput) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inputs = in
}

func TestConsensusProgress_NoStall(t *testing.T) {
	store := NewStore()
	snap := &fakeProgress{inputs: []ConsensusProgressInput{
		{Node: "n0", ClosedSeq: 100, ValidatedSeq: 99},
		{Node: "n1", ClosedSeq: 100, ValidatedSeq: 99},
	}}
	o := NewConsensusProgressOracle(snap, store, time.Hour, 10, 2*time.Minute)

	now := time.Now()
	o.tick(now)
	o.tick(now.Add(5 * time.Minute))

	if store.Len() != 0 {
		t.Fatalf("expected 0 findings, got %d", store.Len())
	}
}

func TestConsensusProgress_SustainedGapFires(t *testing.T) {
	store := NewStore()
	snap := &fakeProgress{inputs: []ConsensusProgressInput{
		{Node: "n0", ClosedSeq: 100, ValidatedSeq: 50}, // gap 50, well over threshold
		{Node: "n1", ClosedSeq: 100, ValidatedSeq: 50},
	}}
	o := NewConsensusProgressOracle(snap, store, time.Hour, 10, 2*time.Minute)

	t0 := time.Now()
	o.tick(t0)
	// Just under the sustain window — still no finding.
	o.tick(t0.Add(1*time.Minute + 59*time.Second))
	if store.Len() != 0 {
		t.Fatalf("expected 0 findings before sustain, got %d", store.Len())
	}
	// Cross the sustain boundary — fires once.
	o.tick(t0.Add(2*time.Minute + 1*time.Second))
	if got := store.Len(); got != 1 {
		t.Fatalf("expected 1 finding, got %d", got)
	}
	findings := store.List(ListOpts{Kind: api.KindConsensusStall, Limit: 10})
	if len(findings) != 1 {
		t.Fatalf("expected 1 consensus_stall, got %d", len(findings))
	}
	if findings[0].Severity != api.SeverityError {
		t.Errorf("severity: got %q want %q", findings[0].Severity, api.SeverityError)
	}
	// Same stall episode — must not double-fire.
	o.tick(t0.Add(5 * time.Minute))
	if got := store.Len(); got != 1 {
		t.Fatalf("duplicate firing: store now has %d findings", got)
	}
}

func TestConsensusProgress_GapBelowThreshold(t *testing.T) {
	store := NewStore()
	snap := &fakeProgress{inputs: []ConsensusProgressInput{
		{Node: "n0", ClosedSeq: 100, ValidatedSeq: 95}, // gap 5, under default 10
	}}
	o := NewConsensusProgressOracle(snap, store, time.Hour, 10, 2*time.Minute)

	t0 := time.Now()
	for i := 0; i < 10; i++ {
		o.tick(t0.Add(time.Duration(i) * time.Minute))
	}
	if store.Len() != 0 {
		t.Fatalf("expected 0 findings, got %d", store.Len())
	}
}

func TestConsensusProgress_RecoveryAllowsNewFinding(t *testing.T) {
	store := NewStore()
	snap := &fakeProgress{inputs: []ConsensusProgressInput{
		{Node: "n0", ClosedSeq: 100, ValidatedSeq: 50},
	}}
	o := NewConsensusProgressOracle(snap, store, time.Hour, 10, 2*time.Minute)

	t0 := time.Now()
	o.tick(t0)
	o.tick(t0.Add(3 * time.Minute))
	if store.Len() != 1 {
		t.Fatalf("expected 1 finding after first stall, got %d", store.Len())
	}

	// Recover: gap closes.
	snap.set([]ConsensusProgressInput{{Node: "n0", ClosedSeq: 200, ValidatedSeq: 199}})
	o.tick(t0.Add(4 * time.Minute))
	if store.Len() != 1 {
		t.Fatalf("recovery should not emit; got %d", store.Len())
	}

	// New stall episode begins.
	snap.set([]ConsensusProgressInput{{Node: "n0", ClosedSeq: 250, ValidatedSeq: 200}})
	o.tick(t0.Add(5 * time.Minute))
	o.tick(t0.Add(8 * time.Minute)) // > sustain after the new onset
	if store.Len() != 2 {
		t.Fatalf("expected 2 findings (one per episode), got %d", store.Len())
	}
}

func TestConsensusProgress_EmptySnapshot(t *testing.T) {
	store := NewStore()
	snap := &fakeProgress{}
	o := NewConsensusProgressOracle(snap, store, time.Hour, 10, 2*time.Minute)

	for i := 0; i < 5; i++ {
		o.tick(time.Now().Add(time.Duration(i) * time.Minute))
	}
	if store.Len() != 0 {
		t.Fatalf("expected 0 findings on empty snapshot, got %d", store.Len())
	}
}

func TestConsensusProgress_PartialStallStillFires(t *testing.T) {
	// One node stalls, others fine. The oracle should still emit — partial
	// consensus stalls are real (a single validator stuck causes the whole
	// quorum to grind).
	store := NewStore()
	snap := &fakeProgress{inputs: []ConsensusProgressInput{
		{Node: "n0", ClosedSeq: 200, ValidatedSeq: 199},
		{Node: "n1", ClosedSeq: 200, ValidatedSeq: 100}, // gap 100
	}}
	o := NewConsensusProgressOracle(snap, store, time.Hour, 10, 2*time.Minute)

	t0 := time.Now()
	o.tick(t0)
	o.tick(t0.Add(3 * time.Minute))
	if store.Len() != 1 {
		t.Fatalf("expected 1 finding, got %d", store.Len())
	}
}
