package generator

import (
	mathrand "math/rand/v2"
	"strconv"
)

var trustSetCurrencies = []string{"USD", "EUR", "GBP", "JPY", "BTC"}

// TrustSet generates a well-formed TrustSet between two pool accounts.
// Account trusts Issuer for a random 3-letter currency up to `value`.
func (g *Generator) TrustSet(r *mathrand.Rand) (*Tx, error) {
	acct, issuer := g.pool.PickTwoDistinct(r)
	currency := trustSetCurrencies[r.IntN(len(trustSetCurrencies))]
	limit := strconv.Itoa(r.IntN(10_000) + 1)

	return &Tx{
		TransactionType: "TrustSet",
		Account:         acct.ClassicAddress,
		LimitAmount: map[string]any{
			"currency": currency,
			"issuer":   issuer.ClassicAddress,
			"value":    limit,
		},
		Secret: acct.Seed,
	}, nil
}
