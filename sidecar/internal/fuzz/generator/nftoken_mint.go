package generator

import (
	mathrand "math/rand/v2"
)

// NFTokenMint mints a transferable NFToken with a random taxon and URI. The
// minter is also the issuer, so account_nfts on the minter (run by the runner)
// surfaces the new NFTokenID for NFTokenBurn / NFTokenCreateOffer.
//
// All NFToken types gate on NonFungibleTokensV1_1 rather than the original
// NonFungibleTokensV1: the latter is retired (always-on, so it never appears
// in the live `feature` RPC set the selector inspects), making V1_1 the
// votable amendment the gating can actually observe.
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
