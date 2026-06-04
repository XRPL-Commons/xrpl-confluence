package generator

import (
	mathrand "math/rand/v2"
	"strconv"
)

// AMM deposit/withdraw mode flags (subset the generator uses).
const (
	tfWithdrawAll uint32 = 0x00020000
	tfSingleAsset uint32 = 0x00080000
)

// xrpAsset is the AMM asset specifier for XRP.
func xrpAsset() map[string]any { return map[string]any{"currency": "XRP"} }

// iouAsset is the AMM asset specifier for an issued currency.
func iouAsset(currency, issuer string) map[string]any {
	return map[string]any{"currency": currency, "issuer": issuer}
}

// AMMCreate creates an XRP/USD AMM pool. The creator funds both legs (the dense
// USD mesh guarantees it holds the issuer's USD) and becomes the sole LP token
// holder, which Vote/Bid/Withdraw rely on. Picking the issuer among pool
// accounts yields up to one distinct pool per issuer. Gated by the AMM
// amendment.
func (g *Generator) AMMCreate(r *mathrand.Rand) (*Tx, error) {
	creator, issuer := g.pool.PickTwoDistinct(r)
	currency := trustSetCurrencies[r.IntN(len(trustSetCurrencies))]
	amount := strconv.FormatUint(uint64(r.IntN(41)+10)*1_000_000, 10)
	amount2 := map[string]any{
		"currency": currency,
		"issuer":   issuer.ClassicAddress,
		"value":    strconv.Itoa(r.IntN(41) + 10),
	}
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "AMMCreate",
			"Account":         creator.ClassicAddress,
			"Amount":          amount,
			"Amount2":         amount2,
			"TradingFee":      uint32(r.IntN(1001)),
		},
		Secret: creator.Seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "AMMCreate",
		RequiresAll:     []string{"AMM"},
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.AMMCreate(r) },
	})
}
