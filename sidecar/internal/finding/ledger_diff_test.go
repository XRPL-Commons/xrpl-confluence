package finding

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
)

type fakeFetcher struct {
	byNode map[string]*LedgerSnapshot
	errs   map[string]error
}

func (f *fakeFetcher) FetchLedger(_ context.Context, node string, seq int) (*LedgerSnapshot, error) {
	if err, ok := f.errs[node]; ok {
		return nil, err
	}
	s := f.byNode[node]
	if s == nil {
		return nil, errors.New("no snapshot")
	}
	copy := *s
	copy.Node = node
	copy.Seq = seq
	return &copy, nil
}

func TestComputeLedgerDiff_TxOnlyOnOneNode(t *testing.T) {
	snaps := []*LedgerSnapshot{
		{
			Node: "n0", Seq: 5,
			LedgerHash: "AAAA", AccountHash: "S1", TransactionHash: "T1",
			Transactions: []LedgerTxSnapshot{
				{Hash: "TX-COMMON", TransactionType: "Payment"},
				{Hash: "TX-ONLY-N0", TransactionType: "OfferCreate"},
			},
		},
		{
			Node: "n1", Seq: 5,
			LedgerHash: "BBBB", AccountHash: "S2", TransactionHash: "T2",
			Transactions: []LedgerTxSnapshot{
				{Hash: "TX-COMMON", TransactionType: "Payment"},
			},
		},
	}
	diff := computeLedgerDiff(5, snaps, nil)
	if !diff.RootHashConflict {
		t.Error("expected RootHashConflict")
	}
	if got := diff.CommonHashes; !reflect.DeepEqual(got, []string{"TX-COMMON"}) {
		t.Errorf("CommonHashes: got %v", got)
	}
	if got := diff.OnlyOnNodes["n0"]; !reflect.DeepEqual(got, []string{"TX-ONLY-N0"}) {
		t.Errorf("OnlyOnNodes[n0]: got %v", got)
	}
	if _, ok := diff.OnlyOnNodes["n1"]; ok {
		t.Errorf("OnlyOnNodes[n1] should be absent")
	}
	if !reflect.DeepEqual(diff.SuspectTxTypes, []string{"OfferCreate"}) {
		t.Errorf("SuspectTxTypes: got %v", diff.SuspectTxTypes)
	}
}

func TestComputeLedgerDiff_SameTxsDifferentMeta(t *testing.T) {
	// Both nodes have the same tx hash but the root hashes still differ. The
	// diff should still flag RootHashConflict and have no suspect tx types
	// (since the tx set itself agrees — the disagreement is downstream in
	// meta/state).
	snaps := []*LedgerSnapshot{
		{Node: "a", LedgerHash: "AAAA", AccountHash: "X", TransactionHash: "Y",
			Transactions: []LedgerTxSnapshot{{Hash: "TX1", TransactionType: "Payment"}}},
		{Node: "b", LedgerHash: "AAAA", AccountHash: "X2", TransactionHash: "Y",
			Transactions: []LedgerTxSnapshot{{Hash: "TX1", TransactionType: "Payment"}}},
	}
	diff := computeLedgerDiff(5, snaps, nil)
	if !diff.RootHashConflict {
		t.Error("expected RootHashConflict (account_hash differs)")
	}
	if len(diff.SuspectTxTypes) != 0 {
		t.Errorf("SuspectTxTypes: got %v want empty", diff.SuspectTxTypes)
	}
	if !reflect.DeepEqual(diff.CommonHashes, []string{"TX1"}) {
		t.Errorf("CommonHashes: got %v", diff.CommonHashes)
	}
}

func TestComputeLedgerDiff_FetchError(t *testing.T) {
	snaps := []*LedgerSnapshot{
		{Node: "n0", Seq: 5, LedgerHash: "X"},
		nil, // n1 errored
	}
	errs := map[string]string{"n1": "context deadline exceeded"}
	diff := computeLedgerDiff(5, snaps, errs)
	if diff.FetchErrors["n1"] == "" {
		t.Error("FetchErrors[n1] should be preserved")
	}
	if len(diff.Snapshots) != 1 {
		t.Errorf("Snapshots: got %d, want 1 (nils filtered)", len(diff.Snapshots))
	}
}

func TestDivergenceOracle_AttachesLedgerDiff(t *testing.T) {
	store := NewStore()
	snap := fakeSnap{inputs: []DivergenceInput{
		{Node: "n0", Seq: 7, Hash: "AAAA1111"},
		{Node: "n1", Seq: 7, Hash: "BBBB2222"},
	}}
	fetcher := &fakeFetcher{byNode: map[string]*LedgerSnapshot{
		"n0": {
			LedgerHash: "AAAA1111", AccountHash: "SA", TransactionHash: "TA",
			Transactions: []LedgerTxSnapshot{
				{Hash: "TX1", TransactionType: "Payment"},
				{Hash: "TX-EVIL", TransactionType: "EscrowFinish"},
			},
		},
		"n1": {
			LedgerHash: "BBBB2222", AccountHash: "SB", TransactionHash: "TB",
			Transactions: []LedgerTxSnapshot{
				{Hash: "TX1", TransactionType: "Payment"},
			},
		},
	}}

	oracle := NewDivergenceOracle(snap, store, 50*time.Millisecond).
		WithLedgerFetcher(fetcher)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	oracle.Start(ctx)

	findings := waitForFindings(t, store, 1, time.Second)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	f := findings[0]
	if len(f.Detail) == 0 {
		t.Fatal("Detail not set — fetcher path didn't run")
	}
	var diff LedgerDiff
	if err := json.Unmarshal(f.Detail, &diff); err != nil {
		t.Fatalf("Detail not LedgerDiff JSON: %v", err)
	}
	if diff.Seq != 7 {
		t.Errorf("diff seq: got %d want 7", diff.Seq)
	}
	if !diff.RootHashConflict {
		t.Error("expected RootHashConflict")
	}
	if !reflect.DeepEqual(diff.SuspectTxTypes, []string{"EscrowFinish"}) {
		t.Errorf("SuspectTxTypes: got %v", diff.SuspectTxTypes)
	}
	sort.Strings(f.SuspectedComponents)
	if !reflect.DeepEqual(f.SuspectedComponents, []string{"EscrowFinish"}) {
		t.Errorf("SuspectedComponents: got %v", f.SuspectedComponents)
	}
	if f.Kind != api.KindStateDivergence {
		t.Errorf("kind: got %q", f.Kind)
	}
}

func TestDivergenceOracle_FetcherUnreachable(t *testing.T) {
	// A fetcher that always errors must not prevent the finding from being
	// emitted — the divergence is still real, the diff just lacks snapshots.
	store := NewStore()
	snap := fakeSnap{inputs: []DivergenceInput{
		{Node: "n0", Seq: 7, Hash: "A"},
		{Node: "n1", Seq: 7, Hash: "B"},
	}}
	fetcher := &fakeFetcher{
		byNode: map[string]*LedgerSnapshot{},
		errs:   map[string]error{"n0": errors.New("boom"), "n1": errors.New("boom")},
	}

	oracle := NewDivergenceOracle(snap, store, 50*time.Millisecond).WithLedgerFetcher(fetcher)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	oracle.Start(ctx)

	findings := waitForFindings(t, store, 1, time.Second)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding even when fetcher fails, got %d", len(findings))
	}
	var diff LedgerDiff
	if err := json.Unmarshal(findings[0].Detail, &diff); err != nil {
		t.Fatalf("Detail not LedgerDiff JSON: %v", err)
	}
	if len(diff.FetchErrors) != 2 {
		t.Errorf("FetchErrors: got %v", diff.FetchErrors)
	}
}
