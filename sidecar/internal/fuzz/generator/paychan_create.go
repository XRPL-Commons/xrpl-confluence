package generator

import (
	mathrand "math/rand/v2"
	"strconv"
)

// PaymentChannelCreate opens an XRP payment channel from one pool account to
// another, funding it with 10..50 XRP. PublicKey is the source's signing key
// (the channel's claim-verification key). The runner records the channel so
// PaymentChannelFund / PaymentChannelClaim can reference it. PayChan is a
// retired (always-on) amendment, so this needs no gating.
func (g *Generator) PaymentChannelCreate(r *mathrand.Rand) (*Tx, error) {
	src, dst := g.pool.PickTwoDistinct(r)
	amount := strconv.FormatUint(uint64(r.IntN(41)+10)*1_000_000, 10)
	settleDelay := uint32(r.IntN(86400) + 1)
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "PaymentChannelCreate",
			"Account":         src.ClassicAddress,
			"Destination":     dst.ClassicAddress,
			"Amount":          amount,
			"SettleDelay":     settleDelay,
			"PublicKey":       src.PublicKey,
		},
		Secret: src.Seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "PaymentChannelCreate",
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.PaymentChannelCreate(r) },
	})
}
