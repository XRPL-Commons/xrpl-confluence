package generator

import (
	mathrand "math/rand/v2"
)

// ledgerFixTypeNFTokenPageLink is the only defined LedgerFixType: repair an
// account's NFTokenPage directory links. Owner is required for this type.
const ledgerFixTypeNFTokenPageLink uint32 = 1

// LedgerStateFix submits the NFTokenPageLink ledger-state repair for a pool
// account. It is well-formed whether or not the owner's NFTokenPages actually
// need fixing — a no-op repair is still a useful cross-node result-code signal.
// Gated by the fixNFTokenPageLinks amendment.
func (g *Generator) LedgerStateFix(r *mathrand.Rand) (*Tx, error) {
	acct, owner := g.pool.PickTwoDistinct(r)
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "LedgerStateFix",
			"Account":         acct.ClassicAddress,
			"LedgerFixType":   ledgerFixTypeNFTokenPageLink,
			"Owner":           owner.ClassicAddress,
		},
		Secret: acct.Seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "LedgerStateFix",
		RequiresAll:     []string{"fixNFTokenPageLinks"},
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.LedgerStateFix(r) },
	})
}
