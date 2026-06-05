package generator

import (
	mathrand "math/rand/v2"
	"strconv"
)

// CheckCreate writes an XRP check from one pool account to another for a
// SendMax of 1..50 XRP. The runner records the resulting check so CheckCash /
// CheckCancel can reference it. Gated by the Checks amendment.
func (g *Generator) CheckCreate(r *mathrand.Rand) (*Tx, error) {
	src, dst := g.pool.PickTwoDistinct(r)
	sendMax := strconv.FormatUint(uint64(r.IntN(50)+1)*1_000_000, 10)
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "CheckCreate",
			"Account":         src.ClassicAddress,
			"Destination":     dst.ClassicAddress,
			"SendMax":         sendMax,
		},
		Secret: src.Seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "CheckCreate",
		RequiresAll:     []string{"Checks"},
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.CheckCreate(r) },
	})
}
