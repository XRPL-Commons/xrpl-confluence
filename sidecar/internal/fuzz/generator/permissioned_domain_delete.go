package generator

import (
	"fmt"
	mathrand "math/rand/v2"
)

// PermissionedDomainDelete deletes a tracked permissioned domain, signed by its
// owner. Gated by the PermissionedDomains amendment.
func (g *Generator) PermissionedDomainDelete(r *mathrand.Rand) (*Tx, error) {
	ref, ok := g.tracker.Domains().Pick(r)
	if !ok {
		return nil, fmt.Errorf("no tracked domains to delete")
	}
	seed, ok := g.seedFor(ref.Owner)
	if !ok {
		return nil, fmt.Errorf("domain owner %s not in pool", ref.Owner)
	}
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "PermissionedDomainDelete",
			"Account":         ref.Owner,
			"DomainID":        ref.DomainID,
		},
		Secret: seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "PermissionedDomainDelete",
		RequiresAll:     []string{"PermissionedDomains"},
		CanBuild:        func(g *Generator) bool { return g.tracker.Domains().Count() > 0 },
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.PermissionedDomainDelete(r) },
	})
}
