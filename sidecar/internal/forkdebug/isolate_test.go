package forkdebug

import (
	"encoding/json"
	"testing"
)

// TestSubsetOf_PreservesCanonicalOrder pins the bisection contract:
// when callers pick a subset by index, the returned txs must remain
// in canonical apply order. Otherwise downstream replay would
// produce hashes that diverge for an entirely different reason
// than the bug under investigation.
func TestSubsetOf_PreservesCanonicalOrder(t *testing.T) {
	full := &IsolateResult{
		Sequence:   7,
		SourceNode: "rippled-0",
		Transactions: []IsolateTx{
			{Hash: "00aaaa", Raw: json.RawMessage(`{}`)},
			{Hash: "11bbbb", Raw: json.RawMessage(`{}`)},
			{Hash: "22cccc", Raw: json.RawMessage(`{}`)},
			{Hash: "33dddd", Raw: json.RawMessage(`{}`)},
		},
		TxCount: 4,
	}

	// Pick out-of-order indices; SubsetOf must sort + dedupe.
	got, err := SubsetOf(full, []int{2, 0, 2, 1})
	if err != nil {
		t.Fatalf("SubsetOf: %v", err)
	}
	if got.TxCount != 3 {
		t.Errorf("TxCount = %d, want 3 (deduped 2,0,1)", got.TxCount)
	}
	wantHashes := []string{"00aaaa", "11bbbb", "22cccc"}
	for i, h := range wantHashes {
		if got.Transactions[i].Hash != h {
			t.Errorf("tx[%d].Hash = %q, want %q (canonical order broken)",
				i, got.Transactions[i].Hash, h)
		}
	}
}

// TestSubsetOf_OutOfRange catches the common bisection bug of
// picking an index past the end. Returning a clear error here
// beats silently truncating the subset and then chasing a phantom
// divergence.
func TestSubsetOf_OutOfRange(t *testing.T) {
	full := &IsolateResult{
		Transactions: []IsolateTx{{Hash: "00aaaa"}, {Hash: "11bbbb"}},
		TxCount:      2,
	}
	if _, err := SubsetOf(full, []int{0, 5}); err == nil {
		t.Fatal("expected out-of-range error, got nil")
	}
	if _, err := SubsetOf(full, []int{-1}); err == nil {
		t.Fatal("expected negative-index error, got nil")
	}
}

// TestSubsetOf_NilGuard asserts the function fails fast on a nil
// input rather than panicking — the CLI wrapper passes through
// whatever IsolateAtSeq returns, including possible nils on edge
// failure paths.
func TestSubsetOf_NilGuard(t *testing.T) {
	if _, err := SubsetOf(nil, []int{0}); err == nil {
		t.Fatal("expected nil-input error, got nil")
	}
}

// TestSubsetOf_EmptyIndices returns an empty subset, NOT the full
// set. Bisection harnesses use [] as the "apply nothing" baseline
// to confirm the parent state matches before adding txs.
func TestSubsetOf_EmptyIndices(t *testing.T) {
	full := &IsolateResult{
		Transactions: []IsolateTx{{Hash: "00aaaa"}},
		TxCount:      1,
	}
	got, err := SubsetOf(full, []int{})
	if err != nil {
		t.Fatalf("SubsetOf empty: %v", err)
	}
	if got.TxCount != 0 {
		t.Errorf("empty subset TxCount = %d, want 0", got.TxCount)
	}
}
