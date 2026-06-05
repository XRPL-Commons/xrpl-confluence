package generator

import (
	mathrand "math/rand/v2"
)

// DepositPreauth grants (or revokes) deposit pre-authorization from one pool
// account to another. Account toggles either Authorize or Unauthorize for a
// distinct counterparty. Gated by the DepositPreauth amendment.
func (g *Generator) DepositPreauth(r *mathrand.Rand) (*Tx, error) {
	acct, other := g.pool.PickTwoDistinct(r)
	fields := map[string]any{
		"TransactionType": "DepositPreauth",
		"Account":         acct.ClassicAddress,
	}
	if r.IntN(2) == 0 {
		fields["Authorize"] = other.ClassicAddress
	} else {
		fields["Unauthorize"] = other.ClassicAddress
	}
	return &Tx{Fields: fields, Secret: acct.Seed}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "DepositPreauth",
		RequiresAll:     []string{"DepositPreauth"},
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.DepositPreauth(r) },
	})
}
