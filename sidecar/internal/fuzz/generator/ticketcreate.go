package generator

import (
	mathrand "math/rand/v2"
)

// TicketCreate reserves 1..5 tickets for the sender. Each ticket counts
// against owner-count reserve until consumed; staying small avoids reserve
// flapping during the fuzz. XRPL's hard upper bound is 250 per tx.
func (g *Generator) TicketCreate(r *mathrand.Rand) (*Tx, error) {
	acct := g.pool.Pick(r)
	count := uint32(r.IntN(5) + 1)
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "TicketCreate",
			"Account":         acct.ClassicAddress,
			"TicketCount":     count,
		},
		Secret: acct.Seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "TicketCreate",
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.TicketCreate(r) },
	})
}
