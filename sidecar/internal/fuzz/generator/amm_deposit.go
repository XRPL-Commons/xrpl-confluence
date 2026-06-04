package generator

import (
	"fmt"
	mathrand "math/rand/v2"
	"strconv"
)

// AMMDeposit makes a single-asset XRP deposit (1..10 XRP) into a tracked AMM,
// minting LP tokens for the depositor. Single-asset mode avoids pool-ratio
// math, keeping the deposit well-formed. Gated by the AMM amendment.
func (g *Generator) AMMDeposit(r *mathrand.Rand) (*Tx, error) {
	ref, ok := g.tracker.AMMs().Pick(r)
	if !ok {
		return nil, fmt.Errorf("no tracked AMMs to deposit into")
	}
	seed, ok := g.seedFor(ref.Creator)
	if !ok {
		return nil, fmt.Errorf("AMM creator %s not in pool", ref.Creator)
	}
	amount := strconv.FormatUint(uint64(r.IntN(10)+1)*1_000_000, 10)
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "AMMDeposit",
			"Account":         ref.Creator,
			"Asset":           xrpAsset(),
			"Asset2":          iouAsset(ref.Currency, ref.Issuer),
			"Amount":          amount,
			"Flags":           tfSingleAsset,
		},
		Secret: seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "AMMDeposit",
		RequiresAll:     []string{"AMM"},
		CanBuild:        func(g *Generator) bool { return g.tracker.AMMs().Count() > 0 },
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.AMMDeposit(r) },
	})
}
