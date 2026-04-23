package generator

import (
	mathrand "math/rand/v2"
)

// SetRegularKey randomly either sets RegularKey to another pool account, or
// clears it by omitting the field. Production caveat: clearing while asfDisableMaster
// is set blackholes the account — AccountSet generator excludes asfDisableMaster
// for this reason.
func (g *Generator) SetRegularKey(r *mathrand.Rand) (*Tx, error) {
	acct := g.pool.Pick(r)
	fields := map[string]any{
		"TransactionType": "SetRegularKey",
		"Account":         acct.ClassicAddress,
	}
	if r.IntN(2) == 0 {
		other := g.pool.Pick(r)
		for other.ClassicAddress == acct.ClassicAddress {
			other = g.pool.Pick(r)
		}
		fields["RegularKey"] = other.ClassicAddress
	}
	return &Tx{Fields: fields, Secret: acct.Seed}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "SetRegularKey",
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.SetRegularKey(r) },
	})
}
