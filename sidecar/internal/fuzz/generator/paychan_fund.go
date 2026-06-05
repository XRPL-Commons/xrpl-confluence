package generator

import (
	"fmt"
	mathrand "math/rand/v2"
	"strconv"
)

// PaymentChannelFund adds 1..10 XRP to a tracked channel. Only the channel's
// source may fund it. PayChan is retired (always-on), so no gating.
func (g *Generator) PaymentChannelFund(r *mathrand.Rand) (*Tx, error) {
	ref, ok := g.tracker.Channels().Pick(r)
	if !ok {
		return nil, fmt.Errorf("no tracked channels to fund")
	}
	seed, ok := g.seedFor(ref.Source)
	if !ok {
		return nil, fmt.Errorf("channel source %s not in pool", ref.Source)
	}
	amount := strconv.FormatUint(uint64(r.IntN(10)+1)*1_000_000, 10)
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "PaymentChannelFund",
			"Account":         ref.Source,
			"Channel":         ref.Channel,
			"Amount":          amount,
		},
		Secret: seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "PaymentChannelFund",
		CanBuild:        func(g *Generator) bool { return g.tracker.Channels().Count() > 0 },
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.PaymentChannelFund(r) },
	})
}
