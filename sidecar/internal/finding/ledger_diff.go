package finding

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
)

// LedgerSnapshot is the minimal-but-self-contained view of a single node's
// ledger at a specific seq used to produce the divergence diff artifact.
// Avoids leaking rpcclient types into the finding package so tests can stub
// the fetcher without pulling in the HTTP client.
type LedgerSnapshot struct {
	Node            string             `json:"node"`
	Seq             int                `json:"seq"`
	LedgerHash      string             `json:"ledger_hash"`
	AccountHash     string             `json:"account_hash"`
	TransactionHash string             `json:"transaction_hash"`
	Transactions    []LedgerTxSnapshot `json:"transactions"`
}

// LedgerTxSnapshot is one transaction inside a LedgerSnapshot. Meta is held
// verbatim so the consumer (CLI, dashboard, replay tool) can post-process
// AffectedNodes without re-querying the node.
type LedgerTxSnapshot struct {
	Hash            string          `json:"hash"`
	TransactionType string          `json:"transaction_type,omitempty"`
	Account         string          `json:"account,omitempty"`
	Meta            json.RawMessage `json:"meta,omitempty"`
}

// LedgerFetcher resolves a (node, seq) pair into a LedgerSnapshot. The
// divergence oracle calls this concurrently per node; an implementation that
// hits the network must therefore be safe for parallel use.
type LedgerFetcher interface {
	FetchLedger(ctx context.Context, node string, seq int) (*LedgerSnapshot, error)
}

// LedgerDiff is the structured diff artifact attached to a state_divergence
// finding. It captures "what was different about this ledger across nodes"
// in a form a human (or a replay tool) can act on without re-running the
// scenario.
type LedgerDiff struct {
	Seq              int              `json:"seq"`
	Snapshots        []LedgerSnapshot `json:"snapshots"`
	OnlyOnNodes      map[string][]string `json:"only_on_nodes,omitempty"`        // node -> tx hashes only it has
	CommonHashes     []string         `json:"common_hashes,omitempty"`         // tx hashes present on every node
	SuspectTxTypes   []string         `json:"suspect_tx_types,omitempty"`      // union of TransactionType from any tx that isn't on every node
	RootHashConflict bool             `json:"root_hash_conflict"`              // true when ledger/account/transaction hashes differ across nodes
	FetchErrors      map[string]string `json:"fetch_errors,omitempty"`         // node -> error string for any node we couldn't snapshot
}

// computeLedgerDiff folds parallel snapshots into the diff record. Nodes that
// failed to snapshot land in FetchErrors; they don't contribute hashes to the
// suspect-type union since we can't tell which side is missing what.
func computeLedgerDiff(seq int, snaps []*LedgerSnapshot, fetchErrs map[string]string) *LedgerDiff {
	diff := &LedgerDiff{Seq: seq, FetchErrors: fetchErrs}
	for _, s := range snaps {
		if s != nil {
			diff.Snapshots = append(diff.Snapshots, *s)
		}
	}
	sort.Slice(diff.Snapshots, func(i, j int) bool { return diff.Snapshots[i].Node < diff.Snapshots[j].Node })

	if len(diff.Snapshots) < 2 {
		return diff
	}

	// hash counts across nodes; per-hash representative (for TransactionType)
	hashCount := make(map[string]int)
	rep := make(map[string]LedgerTxSnapshot)
	for _, s := range diff.Snapshots {
		seenInThisNode := make(map[string]bool)
		for _, tx := range s.Transactions {
			if seenInThisNode[tx.Hash] {
				continue
			}
			seenInThisNode[tx.Hash] = true
			hashCount[tx.Hash]++
			if _, ok := rep[tx.Hash]; !ok {
				rep[tx.Hash] = tx
			}
		}
	}

	common := []string{}
	for h, c := range hashCount {
		if c == len(diff.Snapshots) {
			common = append(common, h)
		}
	}
	sort.Strings(common)
	diff.CommonHashes = common

	// only_on_nodes: per node, tx hashes present here but not on every node.
	diff.OnlyOnNodes = make(map[string][]string)
	suspectTypes := make(map[string]struct{})
	for _, s := range diff.Snapshots {
		var only []string
		for _, tx := range s.Transactions {
			if hashCount[tx.Hash] < len(diff.Snapshots) {
				only = append(only, tx.Hash)
				if t := rep[tx.Hash].TransactionType; t != "" {
					suspectTypes[t] = struct{}{}
				}
			}
		}
		sort.Strings(only)
		if len(only) > 0 {
			diff.OnlyOnNodes[s.Node] = only
		}
	}
	if len(diff.OnlyOnNodes) == 0 {
		diff.OnlyOnNodes = nil
	}

	diff.SuspectTxTypes = make([]string, 0, len(suspectTypes))
	for t := range suspectTypes {
		diff.SuspectTxTypes = append(diff.SuspectTxTypes, t)
	}
	sort.Strings(diff.SuspectTxTypes)

	// Root-hash conflict: any of the three roots differ across nodes.
	ref := diff.Snapshots[0]
	for _, s := range diff.Snapshots[1:] {
		if s.LedgerHash != ref.LedgerHash ||
			s.AccountHash != ref.AccountHash ||
			s.TransactionHash != ref.TransactionHash {
			diff.RootHashConflict = true
			break
		}
	}
	return diff
}

// fetchAllConcurrent resolves each (node, seq) pair via fetcher in parallel.
// Returns the snapshots in input order (nil slots for nodes that errored) and
// a node -> error map so callers can surface partial diffs even when one
// node was unreachable.
func fetchAllConcurrent(ctx context.Context, fetcher LedgerFetcher, nodes []string, seq int) ([]*LedgerSnapshot, map[string]string) {
	snaps := make([]*LedgerSnapshot, len(nodes))
	errs := make(map[string]string)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i, n := range nodes {
		i, n := i, n
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := fetcher.FetchLedger(ctx, n, seq)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs[n] = err.Error()
				return
			}
			snaps[i] = s
		}()
	}
	wg.Wait()
	return snaps, errs
}
