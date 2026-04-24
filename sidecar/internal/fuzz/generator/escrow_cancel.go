package generator

import (
	"fmt"
	mathrand "math/rand/v2"
)

// EscrowCancel cancels a tracked escrow. For well-formedness we use the
// owner's seed to sign — only the owner (or after CancelAfter) can cancel.
// If the owner isn't in our pool, we fall back to a pool account (tecNO_PERMISSION
// is a legitimate fuzz signal).
func (g *Generator) EscrowCancel(r *mathrand.Rand) (*Tx, error) {
	owner, seq, ok := g.tracker.Escrows().PickOpen(r)
	if !ok {
		return nil, fmt.Errorf("no tracked escrows to cancel")
	}
	var ownerSeed string
	for _, w := range g.pool.All() {
		if w.ClassicAddress == owner {
			ownerSeed = w.Seed
			break
		}
	}
	if ownerSeed == "" {
		submitter := g.pool.Pick(r)
		ownerSeed = submitter.Seed
	}
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "EscrowCancel",
			"Account":         owner,
			"Owner":           owner,
			"OfferSequence":   seq,
		},
		Secret: ownerSeed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "EscrowCancel",
		CanBuild:        func(g *Generator) bool { return g.tracker.Escrows().Count() > 0 },
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.EscrowCancel(r) },
	})
}
