package generator

import (
	"fmt"
	mathrand "math/rand/v2"
)

// AMMDelete requests deletion of a tracked AMM. A non-empty pool yields
// tecAMM_NOT_EMPTY (well-formed), while an emptied pool is incrementally
// deleted. Gated by the AMM amendment.
func (g *Generator) AMMDelete(r *mathrand.Rand) (*Tx, error) {
	ref, ok := g.tracker.AMMs().Pick(r)
	if !ok {
		return nil, fmt.Errorf("no tracked AMMs to delete")
	}
	acct := g.pool.Pick(r)
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "AMMDelete",
			"Account":         acct.ClassicAddress,
			"Asset":           xrpAsset(),
			"Asset2":          iouAsset(ref.Currency, ref.Issuer),
		},
		Secret: acct.Seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "AMMDelete",
		RequiresAll:     []string{"AMM"},
		CanBuild:        func(g *Generator) bool { return g.tracker.AMMs().Count() > 0 },
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.AMMDelete(r) },
	})
}
