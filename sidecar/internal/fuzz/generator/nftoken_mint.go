package generator

import (
	mathrand "math/rand/v2"
)

// NFTokenMint mints a transferable NFToken with a random taxon and URI. The
// minter is also the issuer, so account_nfts on the minter (run by the runner)
// surfaces the new NFTokenID for NFTokenBurn / NFTokenCreateOffer. Gated by the
// NonFungibleTokensV1_1 amendment.
func (g *Generator) NFTokenMint(r *mathrand.Rand) (*Tx, error) {
	acct := g.pool.Pick(r)
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "NFTokenMint",
			"Account":         acct.ClassicAddress,
			"NFTokenTaxon":    uint32(r.IntN(10000)),
			"Flags":           tfTransferable,
			"URI":             randHexBytes(r, 16),
		},
		Secret: acct.Seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "NFTokenMint",
		RequiresAll:     []string{"NonFungibleTokensV1_1"},
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.NFTokenMint(r) },
	})
}
