package finding

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
)

// DivergenceInput is a minimal snapshot of one node's most-recent validated
// ledger. NodePoller maps its full snapshot into this form via DivergenceSnapshot.
type DivergenceInput struct {
	Node string
	Seq  int
	Hash string
}

// Snapshotter is satisfied by any type that can supply a slice of
// DivergenceInput on demand. NodePoller implements this via its
// DivergenceSnapshot method.
type Snapshotter interface {
	DivergenceSnapshot() []DivergenceInput
}

// DivergenceOracle ticks on a fixed interval, inspects the current node
// snapshot, and emits a finding whenever two or more nodes disagree on the
// validated-ledger hash for the same sequence number.
//
// When a LedgerFetcher is configured (WithLedgerFetcher), the oracle also
// snapshots both ledgers at the divergent seq and attaches a structured diff
// (tx-set delta, suspect transaction types, per-node root hashes) as the
// finding's Detail. This is what removes the "go grep kurtosis logs by hand"
// step on every divergence.
type DivergenceOracle struct {
	snapshotter Snapshotter
	store       *Store
	interval    time.Duration
	fetcher     LedgerFetcher

	mu   sync.Mutex
	seen map[string]bool
}

// NewDivergenceOracle creates an oracle. Call Start to begin detection.
func NewDivergenceOracle(s Snapshotter, store *Store, interval time.Duration) *DivergenceOracle {
	return &DivergenceOracle{
		snapshotter: s,
		store:       store,
		interval:    interval,
		seen:        make(map[string]bool),
	}
}

// WithLedgerFetcher attaches a LedgerFetcher so divergence findings include
// snapshots of every node's ledger at the divergent seq plus a tx-set diff
// labeled with the suspect transaction types. Pass nil to disable. Safe to
// call before Start.
func (o *DivergenceOracle) WithLedgerFetcher(f LedgerFetcher) *DivergenceOracle {
	o.fetcher = f
	return o
}

// Start launches the background detection goroutine. It exits when ctx is cancelled.
func (o *DivergenceOracle) Start(ctx context.Context) {
	go o.run(ctx)
}

func (o *DivergenceOracle) run(ctx context.Context) {
	ticker := time.NewTicker(o.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			o.tick(ctx)
		}
	}
}

func (o *DivergenceOracle) tick(ctx context.Context) {
	inputs := o.snapshotter.DivergenceSnapshot()

	// Group inputs by ledger sequence.
	bySeq := make(map[int][]DivergenceInput)
	for _, in := range inputs {
		bySeq[in.Seq] = append(bySeq[in.Seq], in)
	}

	for seq, group := range bySeq {
		// Collect distinct hashes.
		hashSet := make(map[string]struct{})
		for _, in := range group {
			hashSet[in.Hash] = struct{}{}
		}
		if len(hashSet) < 2 {
			continue
		}

		// Build dedup key from sorted distinct hashes.
		hashes := make([]string, 0, len(hashSet))
		for h := range hashSet {
			hashes = append(hashes, h)
		}
		sort.Strings(hashes)
		dedupKey := fmt.Sprintf("%d:%s", seq, strings.Join(hashes, "|"))

		o.mu.Lock()
		already := o.seen[dedupKey]
		if !already {
			o.seen[dedupKey] = true
		}
		o.mu.Unlock()

		if already {
			continue
		}

		f := api.Finding{
			ID:       NewFindingID(),
			Kind:     api.KindStateDivergence,
			Severity: api.SeverityError,
			OpenedAt: time.Now().UTC(),
			Summary: fmt.Sprintf(
				"%d nodes disagree on validated ledger %d: %s",
				len(hashes), seq, summarizeNodesByHash(group),
			),
			Evidence: &api.Evidence{
				LedgerRange: [2]uint32{uint32(seq), uint32(seq)},
				DiffKeys:    []string{fmt.Sprintf("validated_ledger:%d", seq)},
			},
		}

		// If a LedgerFetcher is configured, snapshot every node's ledger at
		// the divergent seq, compute the tx-set diff, and attach the suspect
		// transaction types to the finding. This is the auto-built equivalent
		// of the hand-written standalone-replay.py: the artifact contains
		// everything a reviewer needs to identify which transaction caused
		// the disagreement without grepping container logs.
		if o.fetcher != nil {
			nodeNames := make([]string, 0, len(group))
			for _, in := range group {
				nodeNames = append(nodeNames, in.Node)
			}
			sort.Strings(nodeNames)
			snaps, fetchErrs := fetchAllConcurrent(ctx, o.fetcher, nodeNames, seq)
			diff := computeLedgerDiff(seq, snaps, fetchErrs)
			if raw, err := json.Marshal(diff); err == nil {
				f.Detail = raw
			}
			if len(diff.SuspectTxTypes) > 0 {
				f.SuspectedComponents = append(f.SuspectedComponents, diff.SuspectTxTypes...)
			}
		}

		o.store.Add(f)
	}
}

// summarizeNodesByHash groups nodes by their reported hash and returns a
// human-readable string, e.g. "node-0=A1B2C3D4, node-1=node-2=E5F60718".
func summarizeNodesByHash(inputs []DivergenceInput) string {
	nodesByHash := make(map[string][]string)
	var hashOrder []string
	seen := make(map[string]bool)
	for _, in := range inputs {
		if !seen[in.Hash] {
			seen[in.Hash] = true
			hashOrder = append(hashOrder, in.Hash)
		}
		nodesByHash[in.Hash] = append(nodesByHash[in.Hash], in.Node)
	}
	sort.Strings(hashOrder)

	parts := make([]string, 0, len(hashOrder))
	for _, h := range hashOrder {
		nodes := nodesByHash[h]
		short := h
		if len(short) > 8 {
			short = short[:8]
		}
		parts = append(parts, strings.Join(nodes, "=")+short)
	}
	return strings.Join(parts, ", ")
}
