package generator

import (
	"fmt"
	mathrand "math/rand/v2"
)

// MPTokenIssuanceDestroy destroys a tracked MPToken issuance, signed by its
// issuer. It succeeds only when no holders remain (otherwise a well-formed
// tecHAS_OBLIGATIONS). Gated by the MPTokensV1 amendment.
func (g *Generator) MPTokenIssuanceDestroy(r *mathrand.Rand) (*Tx, error) {
	ref, ok := g.tracker.MPTs().Pick(r)
	if !ok {
		return nil, fmt.Errorf("no tracked MPT issuances to destroy")
	}
	seed, ok := g.seedFor(ref.Issuer)
	if !ok {
		return nil, fmt.Errorf("MPT issuer %s not in pool", ref.Issuer)
	}
	return &Tx{
		Fields: map[string]any{
			"TransactionType":   "MPTokenIssuanceDestroy",
			"Account":           ref.Issuer,
			"MPTokenIssuanceID": ref.IssuanceID,
		},
		Secret: seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "MPTokenIssuanceDestroy",
		RequiresAll:     []string{"MPTokensV1"},
		CanBuild:        func(g *Generator) bool { return g.tracker.MPTs().Count() > 0 },
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.MPTokenIssuanceDestroy(r) },
	})
}
