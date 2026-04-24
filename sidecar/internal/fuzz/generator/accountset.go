package generator

import (
	mathrand "math/rand/v2"
)

// asfFlags is the set of asf* flags a well-formed AccountSet can toggle.
// asfDisableMaster (4) is excluded — enabling it without a regular key
// permanently blackholes the account.
var asfFlags = []uint32{1, 2, 3, 5, 6, 7, 8, 9}

// AccountSet generates a well-formed AccountSet tx that toggles a random asf flag.
func (g *Generator) AccountSet(r *mathrand.Rand) (*Tx, error) {
	acct := g.pool.Pick(r)
	flag := asfFlags[r.IntN(len(asfFlags))]
	fields := map[string]any{
		"TransactionType": "AccountSet",
		"Account":         acct.ClassicAddress,
	}
	if r.IntN(2) == 0 {
		fields["SetFlag"] = flag
	} else {
		fields["ClearFlag"] = flag
	}
	return &Tx{Fields: fields, Secret: acct.Seed}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "AccountSet",
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.AccountSet(r) },
	})
}
