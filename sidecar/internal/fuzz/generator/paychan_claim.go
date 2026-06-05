package generator

import (
	"fmt"
	mathrand "math/rand/v2"
)

// PaymentChannelClaim has the channel's source set a Balance, delivering 1 XRP
// to the destination — a claim the source can authorize on its own (no
// counterparty signature needed). PayChan is retired (always-on), so no gating.
func (g *Generator) PaymentChannelClaim(r *mathrand.Rand) (*Tx, error) {
	ref, ok := g.tracker.Channels().Pick(r)
	if !ok {
		return nil, fmt.Errorf("no tracked channels to claim")
	}
	seed, ok := g.seedFor(ref.Source)
	if !ok {
		return nil, fmt.Errorf("channel source %s not in pool", ref.Source)
	}
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "PaymentChannelClaim",
			"Account":         ref.Source,
			"Channel":         ref.Channel,
			"Balance":         "1000000",
		},
		Secret: seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "PaymentChannelClaim",
		CanBuild:        func(g *Generator) bool { return g.tracker.Channels().Count() > 0 },
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.PaymentChannelClaim(r) },
	})
}
