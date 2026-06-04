package generator

import (
	"fmt"
	mathrand "math/rand/v2"
)

// CredentialDelete removes a tracked credential, signed by its issuer (who may
// always delete). Gated by the Credentials amendment.
func (g *Generator) CredentialDelete(r *mathrand.Rand) (*Tx, error) {
	ref, ok := g.tracker.Credentials().Pick(r)
	if !ok {
		return nil, fmt.Errorf("no tracked credentials to delete")
	}
	seed, ok := g.seedFor(ref.Issuer)
	if !ok {
		return nil, fmt.Errorf("credential issuer %s not in pool", ref.Issuer)
	}
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "CredentialDelete",
			"Account":         ref.Issuer,
			"Subject":         ref.Subject,
			"CredentialType":  ref.CredentialType,
		},
		Secret: seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "CredentialDelete",
		RequiresAll:     []string{"Credentials"},
		CanBuild:        func(g *Generator) bool { return g.tracker.Credentials().Count() > 0 },
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.CredentialDelete(r) },
	})
}
