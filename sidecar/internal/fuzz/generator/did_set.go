package generator

import (
	mathrand "math/rand/v2"
)

// DIDSet creates or updates the sender's DID with a random URI. The runner
// records the owner so DIDDelete can reference it. Gated by the DID amendment.
func (g *Generator) DIDSet(r *mathrand.Rand) (*Tx, error) {
	acct := g.pool.Pick(r)
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "DIDSet",
			"Account":         acct.ClassicAddress,
			"URI":             randHexBytes(r, 16),
		},
		Secret: acct.Seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "DIDSet",
		RequiresAll:     []string{"DID"},
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.DIDSet(r) },
	})
}
