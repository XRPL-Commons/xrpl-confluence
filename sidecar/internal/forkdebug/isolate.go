package forkdebug

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// IsolateResult holds the tx-set fetched from a "healthy" node at a
// known fork seq. The CLI emits this as JSON so a downstream
// bisection harness can replay subsets onto the diverging node.
type IsolateResult struct {
	Sequence       int           `json:"sequence"`
	SourceNode     string        `json:"source_node"`
	LedgerHash     string        `json:"ledger_hash"`
	AccountHash    string        `json:"account_hash"`
	ParentHash     string        `json:"parent_hash"`
	TransactionRoot string       `json:"transaction_hash"`
	CloseTime      int64         `json:"close_time"`
	CloseFlags     int           `json:"close_flags"`
	TxCount        int           `json:"tx_count"`
	// Transactions are presented in canonical (tx-hash ascending)
	// order, matching how rippled and goxrpl walk the tx-set
	// SHAMap during apply. Bisection by index therefore has
	// well-defined semantics: a subset [0..k] applies the first
	// k+1 txs in canonical apply order.
	Transactions []IsolateTx `json:"transactions"`
}

// IsolateTx is one transaction from the divergent ledger. We keep
// the full raw JSON so the downstream tool can serialize/sign/
// resubmit without losing fields.
type IsolateTx struct {
	Hash            string          `json:"hash"`
	TransactionType string          `json:"transaction_type"`
	Account         string          `json:"account"`
	Sequence        uint32          `json:"sequence,omitempty"`
	Fee             string          `json:"fee,omitempty"`
	Raw             json.RawMessage `json:"raw"`
}

// IsolateAtSeq fetches the full ledger (with expanded transactions)
// from `node` and returns it in a form the bisection harness can
// consume. Pick a node KNOWN to have the canonical version of seq
// — usually the rippled side, which acts as the spec reference.
//
// Returns an error if the node does not have the seq, or if the
// ledger has zero transactions (nothing to bisect; the divergence
// is consensus-driven not execution-driven, see seq=18 close-time
// stall in the issue #401 investigation).
func IsolateAtSeq(node Node, seq int) (*IsolateResult, error) {
	if node.URL == "" {
		return nil, errors.New("isolate: empty node URL")
	}
	c := rpcclient.New(node.URL)
	raw, err := c.Call("ledger", map[string]interface{}{
		"ledger_index":  seq,
		"transactions":  true,
		"expand":        true,
	})
	if err != nil {
		return nil, fmt.Errorf("ledger %d via %s: %w", seq, node.Name, err)
	}

	var wrapper struct {
		Ledger struct {
			LedgerHash      string            `json:"ledger_hash"`
			AccountHash     string            `json:"account_hash"`
			ParentHash      string            `json:"parent_hash"`
			TransactionHash string            `json:"transaction_hash"`
			CloseTime       int64             `json:"close_time"`
			CloseFlags      int               `json:"close_flags"`
			Transactions    []json.RawMessage `json:"transactions"`
		} `json:"ledger"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("isolate parse seq=%d: %w", seq, err)
	}
	if wrapper.Status == "error" {
		return nil, fmt.Errorf("isolate: node %s does not have seq=%d", node.Name, seq)
	}

	out := &IsolateResult{
		Sequence:        seq,
		SourceNode:      node.Name,
		LedgerHash:      wrapper.Ledger.LedgerHash,
		AccountHash:     wrapper.Ledger.AccountHash,
		ParentHash:      wrapper.Ledger.ParentHash,
		TransactionRoot: wrapper.Ledger.TransactionHash,
		CloseTime:       wrapper.Ledger.CloseTime,
		CloseFlags:      wrapper.Ledger.CloseFlags,
	}

	for _, txRaw := range wrapper.Ledger.Transactions {
		var meta struct {
			Hash            string `json:"hash"`
			TransactionType string `json:"TransactionType"`
			Account         string `json:"Account"`
			Sequence        uint32 `json:"Sequence"`
			Fee             string `json:"Fee"`
		}
		// Best-effort field extraction; raw JSON is preserved for
		// any caller that needs more.
		_ = json.Unmarshal(txRaw, &meta)
		out.Transactions = append(out.Transactions, IsolateTx{
			Hash:            meta.Hash,
			TransactionType: meta.TransactionType,
			Account:         meta.Account,
			Sequence:        meta.Sequence,
			Fee:             meta.Fee,
			Raw:             txRaw,
		})
	}
	out.TxCount = len(out.Transactions)

	// Canonical sort: ascending by tx hash. rippled and goxrpl
	// both apply in this order; bisection indices must therefore
	// align with this order.
	sort.Slice(out.Transactions, func(i, j int) bool {
		return out.Transactions[i].Hash < out.Transactions[j].Hash
	})

	return out, nil
}

// SubsetOf returns a copy of r with only the txs at the given
// indices (sorted ascending), useful for bisection: callers build
// a subset, hand-replay it onto the diverging node, and check
// whether the resulting account_hash matches.
//
// Returns nil with an error if any index is out of range. Indices
// are de-duplicated and sorted; the returned slice preserves
// canonical apply order.
func SubsetOf(r *IsolateResult, indices []int) (*IsolateResult, error) {
	if r == nil {
		return nil, errors.New("subset: nil isolate result")
	}
	seen := make(map[int]struct{}, len(indices))
	for _, idx := range indices {
		if idx < 0 || idx >= len(r.Transactions) {
			return nil, fmt.Errorf("subset: index %d out of range [0..%d)", idx, len(r.Transactions))
		}
		seen[idx] = struct{}{}
	}
	keep := make([]int, 0, len(seen))
	for idx := range seen {
		keep = append(keep, idx)
	}
	sort.Ints(keep)

	subset := *r
	subset.Transactions = make([]IsolateTx, 0, len(keep))
	for _, idx := range keep {
		subset.Transactions = append(subset.Transactions, r.Transactions[idx])
	}
	subset.TxCount = len(subset.Transactions)
	return &subset, nil
}
