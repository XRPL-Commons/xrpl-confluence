package generator

import (
	"fmt"
	mathrand "math/rand/v2"
)

// AMMVote votes a new trading fee (0..1%) on a tracked AMM, signed by the
// creator (an LP token holder, which voting requires). Gated by the AMM
// amendment.
func (g *Generator) AMMVote(r *mathrand.Rand) (*Tx, error) {
	ref, ok := g.tracker.AMMs().Pick(r)
	if !ok {
		return nil, fmt.Errorf("no tracked AMMs to vote on")
	}
	seed, ok := g.seedFor(ref.Creator)
	if !ok {
		return nil, fmt.Errorf("AMM creator %s not in pool", ref.Creator)
	}
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "AMMVote",
			"Account":         ref.Creator,
			"Asset":           xrpAsset(),
			"Asset2":          iouAsset(ref.Currency, ref.Issuer),
			"TradingFee":      uint32(r.IntN(1001)),
		},
		Secret: seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "AMMVote",
		RequiresAll:     []string{"AMM"},
		CanBuild:        func(g *Generator) bool { return g.tracker.AMMs().Count() > 0 },
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.AMMVote(r) },
	})
}
