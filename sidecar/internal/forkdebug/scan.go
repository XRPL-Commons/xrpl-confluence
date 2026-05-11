// Package forkdebug provides fork-investigation tooling for mixed
// rippled/goXRPL networks: locate the first divergent ledger seq,
// dump the tx-set at a known fork seq, and tail goXRPL consensus
// logs for close-time vote / mode-change visibility.
//
// Built against lessons from the issue #401 5-validator UNL
// bootstrap stall, where finding the first-divergent seq across 5
// nodes meant tens of manual curl loops, and identifying the
// close-time tie-break bug took grepping per-second `ct-avalanche`
// lines out of docker logs. These tools fold both into a CLI.
package forkdebug

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/oracle"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// Node names a single XRPL RPC endpoint for the scanner.
type Node struct {
	Name string
	URL  string
}

// ScanResult is the result of sweeping a sequence range for the
// first ledger on which any pair of nodes disagrees on any of
// ledger_hash / account_hash / transaction_hash.
//
// FirstFork.Sequence == 0 with FirstFork == nil means no divergence
// was observed in the swept range (or every node was missing at
// least one seq, see Errors).
type ScanResult struct {
	FromSeq  int                  `json:"from_seq"`
	ToSeq    int                  `json:"to_seq"`
	Compared int                  `json:"compared"`
	// FirstFork is the first ledger seq where any pair of nodes
	// disagreed on any root hash. Nil if the swept range was fully
	// consistent (or no comparisons could complete).
	FirstFork *oracle.LedgerComparison `json:"first_fork,omitempty"`
	// LastAgreed is the highest seq < FirstFork.Sequence where all
	// nodes agreed. Useful to know "fork is between seq=N and N+1".
	LastAgreed *oracle.LedgerComparison `json:"last_agreed,omitempty"`
	Errors     []string                 `json:"errors,omitempty"`
}

// Scanner walks a sequence range and locates the first fork.
type Scanner struct {
	oracle *oracle.Oracle
}

// NewScanner builds a scanner over the given nodes.
func NewScanner(nodes []Node) (*Scanner, error) {
	if len(nodes) < 2 {
		return nil, errors.New("scan needs at least 2 nodes")
	}
	oracleNodes := make([]oracle.Node, 0, len(nodes))
	for _, n := range nodes {
		if n.Name == "" || n.URL == "" {
			return nil, fmt.Errorf("node missing name or URL: %+v", n)
		}
		oracleNodes = append(oracleNodes, oracle.Node{
			Name:   n.Name,
			Client: rpcclient.New(n.URL),
		})
	}
	return &Scanner{oracle: oracle.New(oracleNodes)}, nil
}

// FindFirstFork sweeps [from, to] inclusive and returns the first
// seq where ≥2 nodes that successfully returned a hash actually
// disagree on any root hash.
//
// Per-node availability errors (e.g. rippled returning lgrNotFound
// for very early seqs whose nodestore was pruned, or a node still
// catching up) are tracked in the per-seq Errors but DO NOT count
// as forks: a real fork is "two nodes saw two different hashes",
// not "one node lacks data". Without this distinction the very
// first sweep against a freshly-started network reports a phantom
// fork at seq=1 because not every node persists the genesis row
// in a way the `ledger` RPC will surface.
//
// On the first real divergence sweep stops — the goal is to
// surface the EARLIEST fork so callers can drill in. Sweep order
// is ascending so LastAgreed is the immediate ancestor of
// FirstFork; that neighbor pair is what fork-isolation needs as
// its baseline.
func (s *Scanner) FindFirstFork(ctx context.Context, from, to int) *ScanResult {
	if from < 1 {
		from = 1
	}
	if to < from {
		to = from
	}
	result := &ScanResult{FromSeq: from, ToSeq: to}

	for seq := from; seq <= to; seq++ {
		if ctx.Err() != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("cancelled at seq=%d: %v", seq, ctx.Err()))
			return result
		}
		cmp := s.oracle.CompareAtSequence(ctx, seq)
		result.Compared++

		// Real fork = ≥2 successful responses that disagree.
		// Availability gaps (some nodes erroring) get logged but
		// don't trigger fork detection.
		if isRealFork(cmp) {
			result.FirstFork = cmp
			return result
		}
		// If at least one pair successfully agreed, treat this seq
		// as the new baseline — even if some other nodes were
		// missing at this height. Otherwise a sparse-availability
		// run would report LastAgreed as nil and the operator
		// would lose the "fork is between N and N+1" framing.
		if len(cmp.NodeHashes) >= 1 {
			result.LastAgreed = cmp
		}
	}

	return result
}

// isRealFork distinguishes a true fork (two responding nodes saw
// two different ledger/account/tx hashes) from a no-op (only one
// or zero nodes responded with data). Pre-condition: cmp is the
// raw output of oracle.CompareAtSequence, where cmp.Agreed=true
// only if every node responded AND every responding node agreed.
func isRealFork(cmp *oracle.LedgerComparison) bool {
	if cmp == nil || cmp.Agreed {
		return false
	}
	if len(cmp.NodeHashes) < 2 {
		return false
	}
	// Any divergence between two responding nodes counts. The
	// oracle already populates Divergences; if it's non-empty we
	// have two nodes-with-data pointing at different hashes.
	return len(cmp.Divergences) > 0
}

// FormatScanResult renders a ScanResult as a human-readable report.
// Used by the CLI to produce a concise summary; the JSON form is
// the structured artifact.
func FormatScanResult(r *ScanResult) string {
	if r == nil {
		return "(nil scan result)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Scanned seq=[%d..%d] (%d ledgers compared)\n",
		r.FromSeq, r.ToSeq, r.Compared)

	if r.FirstFork == nil {
		fmt.Fprintf(&b, "No fork detected — all nodes agreed on every ledger in range.\n")
		if r.LastAgreed != nil {
			fmt.Fprintf(&b, "Last agreed seq: %d\n", r.LastAgreed.Sequence)
		}
		for _, e := range r.Errors {
			fmt.Fprintf(&b, "  ! %s\n", e)
		}
		return b.String()
	}

	fmt.Fprintf(&b, "FIRST FORK at seq=%d\n", r.FirstFork.Sequence)
	if r.LastAgreed != nil {
		fmt.Fprintf(&b, "  last-agreed seq=%d (fork happened transitioning %d → %d)\n",
			r.LastAgreed.Sequence, r.LastAgreed.Sequence, r.FirstFork.Sequence)
	}
	fmt.Fprintf(&b, "  per-node hashes:\n")
	// Sort by name for stable output.
	sorted := make([]oracle.NodeHash, len(r.FirstFork.NodeHashes))
	copy(sorted, r.FirstFork.NodeHashes)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	for _, nh := range sorted {
		fmt.Fprintf(&b, "    %-12s ledger=%s acct=%s tx=%s\n",
			nh.Name, shortHex(nh.LedgerHash), shortHex(nh.AccountHash), shortHex(nh.TransactionHash))
	}
	if len(r.FirstFork.Divergences) > 0 {
		fmt.Fprintf(&b, "  divergent fields:\n")
		for _, d := range r.FirstFork.Divergences {
			fmt.Fprintf(&b, "    %-16s %s=%s vs %s=%s\n",
				d.Field, d.NodeA, shortHex(d.HashA), d.NodeB, shortHex(d.HashB))
		}
	}
	for _, e := range r.FirstFork.Errors {
		fmt.Fprintf(&b, "  ! %s\n", e)
	}
	for _, e := range r.Errors {
		fmt.Fprintf(&b, "  ! %s\n", e)
	}
	return b.String()
}

// shortHex truncates a hex string to a fixed width for table output
// while preserving identifying digits. Empty input is rendered as "-"
// so missing hashes don't visually align with real ones.
func shortHex(h string) string {
	if h == "" {
		return "-"
	}
	if len(h) <= 16 {
		return h
	}
	return h[:16] + "..."
}
