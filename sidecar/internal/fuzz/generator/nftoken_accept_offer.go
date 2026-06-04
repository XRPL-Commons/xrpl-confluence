package generator

import (
	"fmt"
	mathrand "math/rand/v2"
)

// NFTokenAcceptOffer accepts a tracked sell offer: a buyer (any pool account
// other than the seller) pays the offer's price and receives the NFToken.
// Gated by the NonFungibleTokensV1_1 amendment.
func (g *Generator) NFTokenAcceptOffer(r *mathrand.Rand) (*Tx, error) {
	ref, ok := g.tracker.NFTOffers().Pick(r)
	if !ok || !ref.Sell {
		return nil, fmt.Errorf("no tracked sell offers to accept")
	}
	buyer := g.pool.Pick(r)
	for buyer.ClassicAddress == ref.Owner {
		buyer = g.pool.Pick(r)
	}
	return &Tx{
		Fields: map[string]any{
			"TransactionType":  "NFTokenAcceptOffer",
			"Account":          buyer.ClassicAddress,
			"NFTokenSellOffer": ref.OfferID,
		},
		Secret: buyer.Seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "NFTokenAcceptOffer",
		RequiresAll:     []string{"NonFungibleTokensV1_1"},
		CanBuild:        func(g *Generator) bool { return g.tracker.NFTOffers().Count() > 0 },
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.NFTokenAcceptOffer(r) },
	})
}
