package generator

import (
	"fmt"
	mathrand "math/rand/v2"
)

// NFTokenCancelOffer cancels a tracked NFToken offer, signed by its creator.
// Gated by the NonFungibleTokensV1_1 amendment.
func (g *Generator) NFTokenCancelOffer(r *mathrand.Rand) (*Tx, error) {
	ref, ok := g.tracker.NFTOffers().Pick(r)
	if !ok {
		return nil, fmt.Errorf("no tracked NFToken offers to cancel")
	}
	seed, ok := g.seedFor(ref.Owner)
	if !ok {
		return nil, fmt.Errorf("NFToken offer owner %s not in pool", ref.Owner)
	}
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "NFTokenCancelOffer",
			"Account":         ref.Owner,
			"NFTokenOffers":   []any{ref.OfferID},
		},
		Secret: seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "NFTokenCancelOffer",
		RequiresAll:     []string{"NonFungibleTokensV1_1"},
		CanBuild:        func(g *Generator) bool { return g.tracker.NFTOffers().Count() > 0 },
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.NFTokenCancelOffer(r) },
	})
}
