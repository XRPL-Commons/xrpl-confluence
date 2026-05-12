package finding

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
)

type fakeSnap struct{ inputs []DivergenceInput }

func (f fakeSnap) DivergenceSnapshot() []DivergenceInput { return f.inputs }

func waitForFindings(t *testing.T, store *Store, want int, timeout time.Duration) []api.Finding {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		findings := store.List(ListOpts{Kind: api.KindStateDivergence, Limit: 100})
		if len(findings) >= want {
			return findings
		}
		time.Sleep(10 * time.Millisecond)
	}
	return store.List(ListOpts{Kind: api.KindStateDivergence, Limit: 100})
}

func TestDivergenceOracle_AllAgree(t *testing.T) {
	store := NewStore()
	snap := fakeSnap{inputs: []DivergenceInput{
		{Node: "node-0", Seq: 100, Hash: "AAAA"},
		{Node: "node-1", Seq: 100, Hash: "AAAA"},
		{Node: "node-2", Seq: 100, Hash: "AAAA"},
	}}
	oracle := NewDivergenceOracle(snap, store, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	oracle.Start(ctx)

	time.Sleep(150 * time.Millisecond)
	findings := store.List(ListOpts{Kind: api.KindStateDivergence, Limit: 100})
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %d", len(findings))
	}
}

func TestDivergenceOracle_TwoNodesDiverge(t *testing.T) {
	store := NewStore()
	snap := fakeSnap{inputs: []DivergenceInput{
		{Node: "node-0", Seq: 100, Hash: "AAAA1111"},
		{Node: "node-1", Seq: 100, Hash: "BBBB2222"},
	}}
	oracle := NewDivergenceOracle(snap, store, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	oracle.Start(ctx)

	findings := waitForFindings(t, store, 1, 500*time.Millisecond)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	f := findings[0]
	if f.Kind != api.KindStateDivergence {
		t.Errorf("wrong kind: %s", f.Kind)
	}
	if f.Severity != api.SeverityError {
		t.Errorf("wrong severity: %s", f.Severity)
	}
	if f.Evidence == nil {
		t.Fatal("evidence is nil")
	}
	if f.Evidence.LedgerRange[0] != 100 || f.Evidence.LedgerRange[1] != 100 {
		t.Errorf("wrong ledger range: %v", f.Evidence.LedgerRange)
	}
}

func TestDivergenceOracle_DeduplicatesSameDivergence(t *testing.T) {
	store := NewStore()
	snap := fakeSnap{inputs: []DivergenceInput{
		{Node: "node-0", Seq: 100, Hash: "AAAA1111"},
		{Node: "node-1", Seq: 100, Hash: "BBBB2222"},
	}}
	oracle := NewDivergenceOracle(snap, store, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	oracle.Start(ctx)

	// Wait for the first finding, then wait two more ticks to confirm no duplicates.
	waitForFindings(t, store, 1, 500*time.Millisecond)
	time.Sleep(150 * time.Millisecond)

	findings := store.List(ListOpts{Kind: api.KindStateDivergence, Limit: 100})
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding after dedup, got %d", len(findings))
	}
}

func TestDivergenceOracle_DifferentSeqNewFinding(t *testing.T) {
	store := NewStore()

	// First divergence at seq 100.
	snap := &fakeSnapMutable{inputs: []DivergenceInput{
		{Node: "node-0", Seq: 100, Hash: "AAAA1111"},
		{Node: "node-1", Seq: 100, Hash: "BBBB2222"},
	}}
	oracle := NewDivergenceOracle(snap, store, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	oracle.Start(ctx)

	waitForFindings(t, store, 1, 500*time.Millisecond)

	// Change to a different seq with same kind of disagreement.
	snap.set([]DivergenceInput{
		{Node: "node-0", Seq: 200, Hash: "AAAA1111"},
		{Node: "node-1", Seq: 200, Hash: "BBBB2222"},
	})

	findings := waitForFindings(t, store, 2, 500*time.Millisecond)
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings for different seqs, got %d", len(findings))
	}
}

func TestDivergenceOracle_EmptySnapshot(t *testing.T) {
	store := NewStore()
	snap := fakeSnap{inputs: []DivergenceInput{}}
	oracle := NewDivergenceOracle(snap, store, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	oracle.Start(ctx)

	time.Sleep(150 * time.Millisecond)
	findings := store.List(ListOpts{Kind: api.KindStateDivergence, Limit: 100})
	if len(findings) != 0 {
		t.Fatalf("expected no findings for empty snapshot, got %d", len(findings))
	}
}

func TestDivergenceOracle_SingleNode(t *testing.T) {
	store := NewStore()
	snap := fakeSnap{inputs: []DivergenceInput{
		{Node: "node-0", Seq: 100, Hash: "AAAA1111"},
	}}
	oracle := NewDivergenceOracle(snap, store, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	oracle.Start(ctx)

	time.Sleep(150 * time.Millisecond)
	findings := store.List(ListOpts{Kind: api.KindStateDivergence, Limit: 100})
	if len(findings) != 0 {
		t.Fatalf("expected no findings for single node, got %d", len(findings))
	}
}

// fakeSnapMutable is a thread-safe mutable fake snapshotter for test case 4.
type fakeSnapMutable struct {
	mu     sync.Mutex
	inputs []DivergenceInput
}

func (f *fakeSnapMutable) DivergenceSnapshot() []DivergenceInput {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]DivergenceInput, len(f.inputs))
	copy(out, f.inputs)
	return out
}

func (f *fakeSnapMutable) set(inputs []DivergenceInput) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inputs = inputs
}
