package generator

import (
	mathrand "math/rand/v2"
	"strconv"
)

// EscrowCreate generates a time-based escrow from one pool account to another
// for 1..10 XRP. FinishAfter uses a deterministic large timestamp in XRPL's
// Ripple epoch — enough that the escrow is still holdable through the fuzz
// run but well-formed per rippled's FinishAfter > parent-close-time check.
func (g *Generator) EscrowCreate(r *mathrand.Rand) (*Tx, error) {
	src, dst := g.pool.PickTwoDistinct(r)
	drops := uint64(r.IntN(10)+1) * 1_000_000

	// Ripple epoch: seconds since 2000-01-01. We pick a future-ish value
	// (100000 seconds + a random jitter) deterministically from the RNG.
	finishAfter := uint32(946684800 + 100000 + r.IntN(1000))

	return &Tx{
		Fields: map[string]any{
			"TransactionType": "EscrowCreate",
			"Account":         src.ClassicAddress,
			"Destination":     dst.ClassicAddress,
			"Amount":          strconv.FormatUint(drops, 10),
			"FinishAfter":     finishAfter,
		},
		Secret: src.Seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "EscrowCreate",
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.EscrowCreate(r) },
	})
}
