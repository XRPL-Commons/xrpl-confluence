package generator

import (
	"fmt"
	mathrand "math/rand/v2"
)

// MPTokenIssuanceSet locks or unlocks a tracked MPToken issuance, signed by its
// issuer. The issuance is created lockable, so the flag is honoured. Gated by
// the MPTokensV1 amendment.
func (g *Generator) MPTokenIssuanceSet(r *mathrand.Rand) (*Tx, error) {
	ref, ok := g.tracker.MPTs().Pick(r)
	if !ok {
		return nil, fmt.Errorf("no tracked MPT issuances to set")
	}
	seed, ok := g.seedFor(ref.Issuer)
	if !ok {
		return nil, fmt.Errorf("MPT issuer %s not in pool", ref.Issuer)
	}
	flag := tfMPTUnlock
	if r.IntN(2) == 0 {
		flag = tfMPTLock
	}
	return &Tx{
		Fields: map[string]any{
			"TransactionType":   "MPTokenIssuanceSet",
			"Account":           ref.Issuer,
			"MPTokenIssuanceID": ref.IssuanceID,
			"Flags":             flag,
		},
		Secret: seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "MPTokenIssuanceSet",
		RequiresAll:     []string{"MPTokensV1"},
		CanBuild:        func(g *Generator) bool { return g.tracker.MPTs().Count() > 0 },
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.MPTokenIssuanceSet(r) },
	})
}
