package generator

import (
	"fmt"
	mathrand "math/rand/v2"
	"strconv"
)

// NFTokenCreateOffer creates a sell offer for a tracked NFToken, priced 1..10
// XRP, signed by the NFToken's owner. The offer is open (no Destination), so
// any account can accept it. The runner records the resulting offer so
// NFTokenAcceptOffer / NFTokenCancelOffer can reference it. Gated by the
// NonFungibleTokensV1_1 amendment.
func (g *Generator) NFTokenCreateOffer(r *mathrand.Rand) (*Tx, error) {
	ref, ok := g.tracker.NFTs().Pick(r)
	if !ok {
		return nil, fmt.Errorf("no tracked NFTokens to offer")
	}
	seed, ok := g.seedFor(ref.Owner)
	if !ok {
		return nil, fmt.Errorf("NFToken owner %s not in pool", ref.Owner)
	}
	amount := strconv.FormatUint(uint64(r.IntN(10)+1)*1_000_000, 10)
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "NFTokenCreateOffer",
			"Account":         ref.Owner,
			"NFTokenID":       ref.NFTokenID,
			"Amount":          amount,
			"Flags":           tfSellNFToken,
		},
		Secret: seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "NFTokenCreateOffer",
		RequiresAll:     []string{"NonFungibleTokensV1_1"},
		CanBuild:        func(g *Generator) bool { return g.tracker.NFTs().Count() > 0 },
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.NFTokenCreateOffer(r) },
	})
}
