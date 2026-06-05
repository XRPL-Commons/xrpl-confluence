package generator

import (
	"fmt"
	mathrand "math/rand/v2"
)

// CheckCancel cancels a tracked check, signed by its owner (the CheckCreate
// account, who may always cancel). Gated by the Checks amendment.
func (g *Generator) CheckCancel(r *mathrand.Rand) (*Tx, error) {
	ref, ok := g.tracker.Checks().Pick(r)
	if !ok {
		return nil, fmt.Errorf("no tracked checks to cancel")
	}
	seed, ok := g.seedFor(ref.Owner)
	if !ok {
		return nil, fmt.Errorf("check owner %s not in pool", ref.Owner)
	}
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "CheckCancel",
			"Account":         ref.Owner,
			"CheckID":         ref.CheckID,
		},
		Secret: seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "CheckCancel",
		RequiresAll:     []string{"Checks"},
		CanBuild:        func(g *Generator) bool { return g.tracker.Checks().Count() > 0 },
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.CheckCancel(r) },
	})
}
