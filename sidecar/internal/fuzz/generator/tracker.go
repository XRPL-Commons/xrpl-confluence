package generator

import (
	mathrand "math/rand/v2"
	"sort"
)

// Tracker holds per-family runtime state so the generator can emit tx types
// that require referencing existing ledger objects (e.g. EscrowFinish needs
// an open escrow). The runner feeds the tracker on successful submits; the
// generator consults it when building.
type Tracker struct {
	escrows *EscrowTracker
}

// NewTracker returns a Tracker with every sub-tracker initialized.
func NewTracker() *Tracker {
	return &Tracker{escrows: newEscrowTracker()}
}

// Escrows returns the escrow sub-tracker.
func (t *Tracker) Escrows() *EscrowTracker { return t.escrows }

// EscrowRef identifies one escrow ledger object by its creator account +
// the Sequence the creating tx was assigned.
type EscrowRef struct {
	Owner    string
	Sequence uint32
}

// EscrowTracker records escrow creations so Finish/Cancel can reference them.
// It never removes records — even after Finish or Cancel, the sequence is
// still valid to reference (rippled returns tecNO_TARGET, which is a useful
// fuzz signal). Production-hardening would prune on observed Finish/Cancel.
type EscrowTracker struct {
	refs []EscrowRef
}

func newEscrowTracker() *EscrowTracker {
	return &EscrowTracker{refs: []EscrowRef{}}
}

// Record adds a new escrow reference. Callers must ensure Owner + Sequence
// correspond to an on-ledger EscrowCreate they have already submitted.
func (e *EscrowTracker) Record(owner string, sequence uint32) {
	e.refs = append(e.refs, EscrowRef{Owner: owner, Sequence: sequence})
}

// Count returns the number of recorded escrows.
func (e *EscrowTracker) Count() int {
	return len(e.refs)
}

// PickOpen returns a deterministic random escrow reference, or ok=false if
// none are tracked.
func (e *EscrowTracker) PickOpen(r *mathrand.Rand) (owner string, sequence uint32, ok bool) {
	if len(e.refs) == 0 {
		return "", 0, false
	}
	snap := make([]EscrowRef, len(e.refs))
	copy(snap, e.refs)
	sort.Slice(snap, func(i, j int) bool {
		if snap[i].Owner != snap[j].Owner {
			return snap[i].Owner < snap[j].Owner
		}
		return snap[i].Sequence < snap[j].Sequence
	})
	pick := snap[r.IntN(len(snap))]
	return pick.Owner, pick.Sequence, true
}
