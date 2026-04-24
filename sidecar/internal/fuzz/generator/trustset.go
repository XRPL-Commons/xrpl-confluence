package generator

import (
	mathrand "math/rand/v2"
	"strconv"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/accounts"
)

// trustSetCurrencies is the set of currencies the generator uses when building
// TrustSet and OfferCreate txs. It MUST be a subset of what accounts.SetupState
// seeds, otherwise generated txs will hit tecUNFUNDED_OFFER / tecPATH_DRY.
// M2a seeds USD only; M2b will extend the setup and can grow this list.
var trustSetCurrencies = []string{accounts.SetupCurrency}

// TrustSet generates a well-formed TrustSet between two pool accounts.
// Account trusts Issuer for a random 3-letter currency up to `value`.
func (g *Generator) TrustSet(r *mathrand.Rand) (*Tx, error) {
	acct, issuer := g.pool.PickTwoDistinct(r)
	currency := trustSetCurrencies[r.IntN(len(trustSetCurrencies))]
	limit := strconv.Itoa(r.IntN(10_000) + 1)

	return &Tx{
		Fields: map[string]any{
			"TransactionType": "TrustSet",
			"Account":         acct.ClassicAddress,
			"LimitAmount": map[string]any{
				"currency": currency,
				"issuer":   issuer.ClassicAddress,
				"value":    limit,
			},
		},
		Secret: acct.Seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "TrustSet",
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.TrustSet(r) },
	})
}
