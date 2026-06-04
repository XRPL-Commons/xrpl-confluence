package generator

import (
	"fmt"
	mathrand "math/rand/v2"
)

// MPTokenAuthorize has a holder (any pool account other than the issuer) opt in
// to a tracked MPToken issuance. Gated by the MPTokensV1 amendment.
func (g *Generator) MPTokenAuthorize(r *mathrand.Rand) (*Tx, error) {
	ref, ok := g.tracker.MPTs().Pick(r)
	if !ok {
		return nil, fmt.Errorf("no tracked MPT issuances to authorize")
	}
	holder := g.pool.Pick(r)
	for holder.ClassicAddress == ref.Issuer {
		holder = g.pool.Pick(r)
	}
	return &Tx{
		Fields: map[string]any{
			"TransactionType":   "MPTokenAuthorize",
			"Account":           holder.ClassicAddress,
			"MPTokenIssuanceID": ref.IssuanceID,
		},
		Secret: holder.Seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "MPTokenAuthorize",
		RequiresAll:     []string{"MPTokensV1"},
		CanBuild:        func(g *Generator) bool { return g.tracker.MPTs().Count() > 0 },
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.MPTokenAuthorize(r) },
	})
}
