package generator

import (
	"fmt"
	mathrand "math/rand/v2"
)

// OracleDelete deletes a tracked price oracle, signed by its owner. Gated by
// the PriceOracle amendment.
func (g *Generator) OracleDelete(r *mathrand.Rand) (*Tx, error) {
	ref, ok := g.tracker.Oracles().Pick(r)
	if !ok {
		return nil, fmt.Errorf("no tracked oracles to delete")
	}
	seed, ok := g.seedFor(ref.Owner)
	if !ok {
		return nil, fmt.Errorf("oracle owner %s not in pool", ref.Owner)
	}
	return &Tx{
		Fields: map[string]any{
			"TransactionType":  "OracleDelete",
			"Account":          ref.Owner,
			"OracleDocumentID": ref.DocumentID,
		},
		Secret: seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "OracleDelete",
		RequiresAll:     []string{"PriceOracle"},
		CanBuild:        func(g *Generator) bool { return g.tracker.Oracles().Count() > 0 },
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.OracleDelete(r) },
	})
}
