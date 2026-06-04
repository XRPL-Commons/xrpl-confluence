package generator

import (
	mathrand "math/rand/v2"
	"strconv"
)

// Clawback claws back issued USD from a holder. Per the XRPL design the
// Amount's issuer sub-field names the *holder* being clawed back, while the
// signing Account is the token issuer. The pool's dense USD mesh guarantees the
// holder actually holds the issuer's USD. Issuers don't set
// asfAllowTrustLineClawback here, so this exercises the tecNO_PERMISSION path
// well-formed. Gated by the Clawback amendment.
func (g *Generator) Clawback(r *mathrand.Rand) (*Tx, error) {
	issuer, holder := g.pool.PickTwoDistinct(r)
	currency := trustSetCurrencies[r.IntN(len(trustSetCurrencies))]
	value := strconv.Itoa(r.IntN(1000) + 1)
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "Clawback",
			"Account":         issuer.ClassicAddress,
			"Amount": map[string]any{
				"currency": currency,
				"issuer":   holder.ClassicAddress,
				"value":    value,
			},
		},
		Secret: issuer.Seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "Clawback",
		RequiresAll:     []string{"Clawback"},
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.Clawback(r) },
	})
}
