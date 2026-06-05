package generator

import (
	"fmt"
	mathrand "math/rand/v2"
)

// AMMClawback claws back the issued USD that the AMM creator deposited into a
// tracked pool. The signing Account is the USD issuer; Holder is the creator;
// Asset is the issued leg and Asset2 is XRP. Issuers don't enable
// asfAllowTrustLineClawback here, so this is a well-formed tecNO_PERMISSION
// probe. Gated by the AMM and AMMClawback amendments.
func (g *Generator) AMMClawback(r *mathrand.Rand) (*Tx, error) {
	ref, ok := g.tracker.AMMs().Pick(r)
	if !ok {
		return nil, fmt.Errorf("no tracked AMMs to claw back from")
	}
	seed, ok := g.seedFor(ref.Issuer)
	if !ok {
		return nil, fmt.Errorf("AMM issuer %s not in pool", ref.Issuer)
	}
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "AMMClawback",
			"Account":         ref.Issuer,
			"Holder":          ref.Creator,
			"Asset":           iouAsset(ref.Currency, ref.Issuer),
			"Asset2":          xrpAsset(),
		},
		Secret: seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "AMMClawback",
		RequiresAll:     []string{"AMM", "AMMClawback"},
		CanBuild:        func(g *Generator) bool { return g.tracker.AMMs().Count() > 0 },
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.AMMClawback(r) },
	})
}
