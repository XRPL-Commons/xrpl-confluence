package generator

import (
	"fmt"
	mathrand "math/rand/v2"
)

// AMMWithdraw withdraws all of the creator's LP tokens from a tracked AMM
// (tfWithdrawAll). The creator always holds LP tokens from AMMCreate, so the
// withdraw is well-formed. Gated by the AMM amendment.
func (g *Generator) AMMWithdraw(r *mathrand.Rand) (*Tx, error) {
	ref, ok := g.tracker.AMMs().Pick(r)
	if !ok {
		return nil, fmt.Errorf("no tracked AMMs to withdraw from")
	}
	seed, ok := g.seedFor(ref.Creator)
	if !ok {
		return nil, fmt.Errorf("AMM creator %s not in pool", ref.Creator)
	}
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "AMMWithdraw",
			"Account":         ref.Creator,
			"Asset":           xrpAsset(),
			"Asset2":          iouAsset(ref.Currency, ref.Issuer),
			"Flags":           tfWithdrawAll,
		},
		Secret: seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "AMMWithdraw",
		RequiresAll:     []string{"AMM"},
		CanBuild:        func(g *Generator) bool { return g.tracker.AMMs().Count() > 0 },
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.AMMWithdraw(r) },
	})
}
