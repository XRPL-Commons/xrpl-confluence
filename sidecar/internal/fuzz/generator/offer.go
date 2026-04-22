package generator

import (
	mathrand "math/rand/v2"
	"strconv"
)

// OfferCreate generates a simple offer: sell XRP for a synthetic IOU issued
// by another pool account. This produces real order-book entries that the
// oracle can later compare across nodes.
func (g *Generator) OfferCreate(r *mathrand.Rand) (*Tx, error) {
	acct, issuer := g.pool.PickTwoDistinct(r)
	takerPays := strconv.FormatUint(uint64(r.IntN(100)+1)*1_000_000, 10) // XRP drops
	currency := trustSetCurrencies[r.IntN(len(trustSetCurrencies))]
	takerGets := map[string]any{
		"currency": currency,
		"issuer":   issuer.ClassicAddress,
		"value":    strconv.Itoa(r.IntN(1_000) + 1),
	}
	return &Tx{
		TransactionType: "OfferCreate",
		Account:         acct.ClassicAddress,
		TakerPays:       takerPays,
		TakerGets:       takerGets,
		Secret:          acct.Seed,
	}, nil
}
