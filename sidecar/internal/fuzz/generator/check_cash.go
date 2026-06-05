package generator

import (
	"fmt"
	mathrand "math/rand/v2"
)

// CheckCash cashes a tracked check. Only the check's Destination may cash it,
// for an Amount no greater than the check's SendMax — we cash a fixed 1 XRP,
// always within the 1..50 XRP CheckCreate range. Gated by the Checks amendment.
func (g *Generator) CheckCash(r *mathrand.Rand) (*Tx, error) {
	ref, ok := g.tracker.Checks().Pick(r)
	if !ok {
		return nil, fmt.Errorf("no tracked checks to cash")
	}
	seed, ok := g.seedFor(ref.Destination)
	if !ok {
		return nil, fmt.Errorf("check destination %s not in pool", ref.Destination)
	}
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "CheckCash",
			"Account":         ref.Destination,
			"CheckID":         ref.CheckID,
			"Amount":          "1000000",
		},
		Secret: seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "CheckCash",
		RequiresAll:     []string{"Checks"},
		CanBuild:        func(g *Generator) bool { return g.tracker.Checks().Count() > 0 },
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.CheckCash(r) },
	})
}
