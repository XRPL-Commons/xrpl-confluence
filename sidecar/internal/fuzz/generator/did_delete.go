package generator

import (
	"fmt"
	mathrand "math/rand/v2"
)

// DIDDelete removes a tracked DID, signed by its owner. Gated by the DID
// amendment.
func (g *Generator) DIDDelete(r *mathrand.Rand) (*Tx, error) {
	owner, ok := g.tracker.DIDs().Pick(r)
	if !ok {
		return nil, fmt.Errorf("no tracked DIDs to delete")
	}
	seed, ok := g.seedFor(owner)
	if !ok {
		return nil, fmt.Errorf("DID owner %s not in pool", owner)
	}
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "DIDDelete",
			"Account":         owner,
		},
		Secret: seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "DIDDelete",
		RequiresAll:     []string{"DID"},
		CanBuild:        func(g *Generator) bool { return g.tracker.DIDs().Count() > 0 },
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.DIDDelete(r) },
	})
}
