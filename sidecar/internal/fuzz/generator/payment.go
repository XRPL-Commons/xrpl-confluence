package generator

import (
	mathrand "math/rand/v2"
	"strconv"
)

// Payment generates a well-formed XRP payment between two distinct pool
// accounts. Amount is 1–100 XRP in drops.
func (g *Generator) Payment(r *mathrand.Rand) (*Tx, error) {
	from, to := g.pool.PickTwoDistinct(r)
	amountDrops := uint64(r.IntN(100)+1) * 1_000_000
	return &Tx{
		TransactionType: "Payment",
		Account:         from.ClassicAddress,
		Destination:     to.ClassicAddress,
		Amount:          strconv.FormatUint(amountDrops, 10),
		Secret:          from.Seed,
	}, nil
}
