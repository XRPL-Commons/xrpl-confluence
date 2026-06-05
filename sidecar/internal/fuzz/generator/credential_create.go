package generator

import (
	mathrand "math/rand/v2"
)

// CredentialCreate issues a credential from one pool account (Account, the
// issuer) to another (Subject) with a random CredentialType. The runner records
// it so CredentialAccept / CredentialDelete can reference it. Gated by the
// Credentials amendment.
func (g *Generator) CredentialCreate(r *mathrand.Rand) (*Tx, error) {
	issuer, subject := g.pool.PickTwoDistinct(r)
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "CredentialCreate",
			"Account":         issuer.ClassicAddress,
			"Subject":         subject.ClassicAddress,
			"CredentialType":  randHexBytes(r, 8),
		},
		Secret: issuer.Seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "CredentialCreate",
		RequiresAll:     []string{"Credentials"},
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.CredentialCreate(r) },
	})
}
