package generator

import (
	"fmt"
	mathrand "math/rand/v2"
)

// AMMBid bids for the auction slot on a tracked AMM, signed by the creator (an
// LP token holder). With no BidMin/BidMax it bids the current minimum. Gated by
// the AMM amendment.
func (g *Generator) AMMBid(r *mathrand.Rand) (*Tx, error) {
	ref, ok := g.tracker.AMMs().Pick(r)
	if !ok {
		return nil, fmt.Errorf("no tracked AMMs to bid on")
	}
	seed, ok := g.seedFor(ref.Creator)
	if !ok {
		return nil, fmt.Errorf("AMM creator %s not in pool", ref.Creator)
	}
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "AMMBid",
			"Account":         ref.Creator,
			"Asset":           xrpAsset(),
			"Asset2":          iouAsset(ref.Currency, ref.Issuer),
		},
		Secret: seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "AMMBid",
		RequiresAll:     []string{"AMM"},
		CanBuild:        func(g *Generator) bool { return g.tracker.AMMs().Count() > 0 },
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.AMMBid(r) },
	})
}
