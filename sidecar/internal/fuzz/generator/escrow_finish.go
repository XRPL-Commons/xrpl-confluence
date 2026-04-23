package generator

import (
	"fmt"
	mathrand "math/rand/v2"
)

// EscrowFinish releases a tracked escrow. Without a Condition on the escrow,
// any pool account can finish it (after FinishAfter elapses). Errors if
// tracker has no escrows.
func (g *Generator) EscrowFinish(r *mathrand.Rand) (*Tx, error) {
	owner, seq, ok := g.tracker.Escrows().PickOpen(r)
	if !ok {
		return nil, fmt.Errorf("no tracked escrows to finish")
	}
	submitter := g.pool.Pick(r)
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "EscrowFinish",
			"Account":         submitter.ClassicAddress,
			"Owner":           owner,
			"OfferSequence":   seq,
		},
		Secret: submitter.Seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "EscrowFinish",
		CanBuild:        func(g *Generator) bool { return g.tracker.Escrows().Count() > 0 },
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.EscrowFinish(r) },
	})
}
