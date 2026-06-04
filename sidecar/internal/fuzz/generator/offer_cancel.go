package generator

import (
	"fmt"
	mathrand "math/rand/v2"
)

// OfferCancel cancels a tracked offer. It references the OfferSequence (the
// sequence the OfferCreate was assigned) rather than an object ID, and must be
// signed by the offer's owner.
func (g *Generator) OfferCancel(r *mathrand.Rand) (*Tx, error) {
	ref, ok := g.tracker.Offers().Pick(r)
	if !ok {
		return nil, fmt.Errorf("no tracked offers to cancel")
	}
	seed, ok := g.seedFor(ref.Owner)
	if !ok {
		return nil, fmt.Errorf("offer owner %s not in pool", ref.Owner)
	}
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "OfferCancel",
			"Account":         ref.Owner,
			"OfferSequence":   ref.Sequence,
		},
		Secret: seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "OfferCancel",
		CanBuild:        func(g *Generator) bool { return g.tracker.Offers().Count() > 0 },
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.OfferCancel(r) },
	})
}
