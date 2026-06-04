package generator

import (
	"fmt"
	mathrand "math/rand/v2"
)

// CredentialAccept has the subject accept a tracked credential issued to them.
// Gated by the Credentials amendment.
func (g *Generator) CredentialAccept(r *mathrand.Rand) (*Tx, error) {
	ref, ok := g.tracker.Credentials().Pick(r)
	if !ok {
		return nil, fmt.Errorf("no tracked credentials to accept")
	}
	seed, ok := g.seedFor(ref.Subject)
	if !ok {
		return nil, fmt.Errorf("credential subject %s not in pool", ref.Subject)
	}
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "CredentialAccept",
			"Account":         ref.Subject,
			"Issuer":          ref.Issuer,
			"CredentialType":  ref.CredentialType,
		},
		Secret: seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "CredentialAccept",
		RequiresAll:     []string{"Credentials"},
		CanBuild:        func(g *Generator) bool { return g.tracker.Credentials().Count() > 0 },
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.CredentialAccept(r) },
	})
}
