package generator

import (
	"fmt"
	mathrand "math/rand/v2"
)

// NFTokenModify updates the URI of a tracked NFToken, signed by its issuer
// (the minter, who in this generator is also the owner). Gated by both
// NonFungibleTokensV1_1 and DynamicNFT; the NFToken must have been minted
// mutable for this to succeed, otherwise it is a well-formed tecNO_PERMISSION
// probe of the validation path.
func (g *Generator) NFTokenModify(r *mathrand.Rand) (*Tx, error) {
	ref, ok := g.tracker.NFTs().Pick(r)
	if !ok {
		return nil, fmt.Errorf("no tracked NFTokens to modify")
	}
	seed, ok := g.seedFor(ref.Owner)
	if !ok {
		return nil, fmt.Errorf("NFToken owner %s not in pool", ref.Owner)
	}
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "NFTokenModify",
			"Account":         ref.Owner,
			"NFTokenID":       ref.NFTokenID,
			"URI":             randHexBytes(r, 16),
		},
		Secret: seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "NFTokenModify",
		RequiresAll:     []string{"NonFungibleTokensV1_1", "DynamicNFT"},
		CanBuild:        func(g *Generator) bool { return g.tracker.NFTs().Count() > 0 },
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.NFTokenModify(r) },
	})
}
