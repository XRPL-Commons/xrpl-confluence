package generator

import (
	"fmt"
	mathrand "math/rand/v2"
)

// NFTokenBurn burns a tracked NFToken, signed by its owner. Gated by the
// NonFungibleTokensV1_1 amendment.
func (g *Generator) NFTokenBurn(r *mathrand.Rand) (*Tx, error) {
	ref, ok := g.tracker.NFTs().Pick(r)
	if !ok {
		return nil, fmt.Errorf("no tracked NFTokens to burn")
	}
	seed, ok := g.seedFor(ref.Owner)
	if !ok {
		return nil, fmt.Errorf("NFToken owner %s not in pool", ref.Owner)
	}
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "NFTokenBurn",
			"Account":         ref.Owner,
			"NFTokenID":       ref.NFTokenID,
		},
		Secret: seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "NFTokenBurn",
		RequiresAll:     []string{"NonFungibleTokensV1_1"},
		CanBuild:        func(g *Generator) bool { return g.tracker.NFTs().Count() > 0 },
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.NFTokenBurn(r) },
	})
}
