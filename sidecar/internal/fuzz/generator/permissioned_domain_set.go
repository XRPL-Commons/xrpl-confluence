package generator

import (
	mathrand "math/rand/v2"
)

// PermissionedDomainSet creates a permissioned domain owned by Account, with a
// single accepted-credential rule (issuer + random credential type). Omitting
// DomainID makes a new object whose ID the runner derives from (owner,
// sequence) for PermissionedDomainDelete. Gated by the PermissionedDomains
// amendment.
func (g *Generator) PermissionedDomainSet(r *mathrand.Rand) (*Tx, error) {
	owner, issuer := g.pool.PickTwoDistinct(r)
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "PermissionedDomainSet",
			"Account":         owner.ClassicAddress,
			"AcceptedCredentials": []any{
				map[string]any{
					"Credential": map[string]any{
						"Issuer":         issuer.ClassicAddress,
						"CredentialType": randHexBytes(r, 8),
					},
				},
			},
		},
		Secret: owner.Seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "PermissionedDomainSet",
		RequiresAll:     []string{"PermissionedDomains"},
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.PermissionedDomainSet(r) },
	})
}
