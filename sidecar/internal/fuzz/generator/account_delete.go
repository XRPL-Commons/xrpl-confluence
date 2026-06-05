package generator

import (
	mathrand "math/rand/v2"
)

// AccountDelete generates a well-formed AccountDelete that sends the remaining
// balance to another pool account. In the dense fuzz mesh every account owns
// trust lines, so this reliably hits tecHAS_OBLIGATIONS rather than actually
// removing a pool account — a useful validation-path signal that keeps the
// pool intact. Gated by the DeletableAccounts amendment.
func (g *Generator) AccountDelete(r *mathrand.Rand) (*Tx, error) {
	acct, dest := g.pool.PickTwoDistinct(r)
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "AccountDelete",
			"Account":         acct.ClassicAddress,
			"Destination":     dest.ClassicAddress,
		},
		Secret: acct.Seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "AccountDelete",
		RequiresAll:     []string{"DeletableAccounts"},
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.AccountDelete(r) },
	})
}
