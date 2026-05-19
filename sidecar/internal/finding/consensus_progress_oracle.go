package finding

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
)

// ConsensusProgressInput is a per-node snapshot of "where this node thinks the
// network is" — its highest closed (proposed) ledger vs. its highest validated
// (committed) ledger. The gap between the two is the primary stall signal:
// when nodes keep closing fresh ledgers but never advance the validated marker,
// consensus is failing to converge even though the network looks alive.
type ConsensusProgressInput struct {
	Node         string
	ClosedSeq    int
	ValidatedSeq int
}

// ProgressSnapshotter is satisfied by anything that can supply a slice of
// ConsensusProgressInput on demand. The NodePoller in package server
// implements this via ConsensusProgressSnapshot.
type ProgressSnapshotter interface {
	ConsensusProgressSnapshot() []ConsensusProgressInput
}

// ConsensusProgressOracle ticks on a fixed interval, inspects per-node
// closed/validated snapshots, and emits a single consensus_stall finding when
// closed_seq - validated_seq stays above GapThreshold on any node for at least
// SustainFor. It exists because state_diff only fires once validated_seq
// advances — when consensus never converges, validated_seq is frozen and the
// network is silently broken with no finding emitted. This oracle closes that
// blind spot.
//
// One open finding is emitted per stall episode (across all nodes). The
// episode ends — and a new finding becomes eligible — once any node's gap
// drops back at or below GapThreshold (consensus recovered) or the snapshot
// goes empty (network gone).
type ConsensusProgressOracle struct {
	snapshotter   ProgressSnapshotter
	store         *Store
	interval      time.Duration
	gapThreshold  int
	sustainFor    time.Duration

	mu          sync.Mutex
	stallStarts map[string]time.Time // node -> when its gap first exceeded threshold
	openFinding bool                  // a finding for the current stall episode is already in the store
}

// NewConsensusProgressOracle creates an oracle. Defaults: gap=10, sustain=2m.
// Call Start to begin detection.
func NewConsensusProgressOracle(s ProgressSnapshotter, store *Store, interval time.Duration, gapThreshold int, sustainFor time.Duration) *ConsensusProgressOracle {
	if gapThreshold <= 0 {
		gapThreshold = 10
	}
	if sustainFor <= 0 {
		sustainFor = 2 * time.Minute
	}
	return &ConsensusProgressOracle{
		snapshotter:  s,
		store:        store,
		interval:     interval,
		gapThreshold: gapThreshold,
		sustainFor:   sustainFor,
		stallStarts:  make(map[string]time.Time),
	}
}

// Start launches the background detection goroutine. It exits when ctx is cancelled.
func (o *ConsensusProgressOracle) Start(ctx context.Context) {
	go o.run(ctx)
}

func (o *ConsensusProgressOracle) run(ctx context.Context) {
	ticker := time.NewTicker(o.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			o.tick(time.Now())
		}
	}
}

// tick is the per-iteration body factored out so tests can drive it with
// synthetic timestamps instead of waiting on a real ticker.
func (o *ConsensusProgressOracle) tick(now time.Time) {
	inputs := o.snapshotter.ConsensusProgressSnapshot()

	o.mu.Lock()
	defer o.mu.Unlock()

	if len(inputs) == 0 {
		// Network gone — reset any in-flight stall tracking so a re-emerging
		// network doesn't fire a stale finding the moment it reappears.
		o.stallStarts = make(map[string]time.Time)
		o.openFinding = false
		return
	}

	// Track per-node stall onset. Any node whose gap is currently above the
	// threshold contributes to the worst-case duration; any node back at or
	// below the threshold clears its own tracker.
	var stalled []ConsensusProgressInput
	seen := make(map[string]struct{}, len(inputs))
	worst := time.Duration(0)
	for _, in := range inputs {
		seen[in.Node] = struct{}{}
		gap := in.ClosedSeq - in.ValidatedSeq
		if gap > o.gapThreshold {
			start, ok := o.stallStarts[in.Node]
			if !ok {
				start = now
				o.stallStarts[in.Node] = start
			}
			if d := now.Sub(start); d > worst {
				worst = d
			}
			stalled = append(stalled, in)
		} else {
			delete(o.stallStarts, in.Node)
		}
	}
	// Purge entries for nodes that dropped out of the snapshot entirely.
	for k := range o.stallStarts {
		if _, ok := seen[k]; !ok {
			delete(o.stallStarts, k)
		}
	}

	// Episode lifecycle: every node back under threshold clears the open marker
	// so the next sustained gap fires a fresh finding.
	if len(stalled) == 0 {
		o.openFinding = false
		return
	}

	if o.openFinding || worst < o.sustainFor {
		return
	}

	o.openFinding = true
	f := api.Finding{
		ID:       NewFindingID(),
		Kind:     api.KindConsensusStall,
		Severity: api.SeverityError,
		OpenedAt: now.UTC(),
		Summary:  summarizeStall(stalled, worst, o.gapThreshold),
		Evidence: &api.Evidence{
			DiffKeys: stallDiffKeys(stalled),
		},
	}
	o.store.Add(f)
}

func summarizeStall(stalled []ConsensusProgressInput, worst time.Duration, gapThreshold int) string {
	sorted := append([]ConsensusProgressInput(nil), stalled...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Node < sorted[j].Node })
	parts := make([]string, 0, len(sorted))
	for _, in := range sorted {
		parts = append(parts, fmt.Sprintf("%s closed=%d validated=%d gap=%d",
			in.Node, in.ClosedSeq, in.ValidatedSeq, in.ClosedSeq-in.ValidatedSeq))
	}
	joined := ""
	for i, p := range parts {
		if i > 0 {
			joined += "; "
		}
		joined += p
	}
	return fmt.Sprintf("consensus stalled for %.0fs (gap > %d on %d node(s)): %s",
		worst.Seconds(), gapThreshold, len(stalled), joined)
}

func stallDiffKeys(stalled []ConsensusProgressInput) []string {
	out := make([]string, 0, len(stalled))
	for _, in := range stalled {
		out = append(out, fmt.Sprintf("%s:closed=%d:validated=%d", in.Node, in.ClosedSeq, in.ValidatedSeq))
	}
	sort.Strings(out)
	return out
}
