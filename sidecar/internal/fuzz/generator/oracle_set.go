package generator

import (
	"fmt"
	mathrand "math/rand/v2"
	"time"
)

// OracleSet creates or updates a price oracle with a single XRP/USD price
// point. LastUpdateTime must sit within 300s of the ledger close time, so it is
// stamped from wall-clock time (the enclave tracks real time) rather than the
// RNG — the one builder that is intentionally non-deterministic. AssetPrice is
// emitted as an uppercase hex string, the canonical UInt64 JSON form both
// rippled and goXRPL parse identically. The runner records (owner, documentID)
// so OracleDelete can reference it. Gated by the PriceOracle amendment.
func (g *Generator) OracleSet(r *mathrand.Rand) (*Tx, error) {
	acct := g.pool.Pick(r)
	docID := uint32(r.IntN(1000))
	price := uint64(r.IntN(100000) + 1)
	return &Tx{
		Fields: map[string]any{
			"TransactionType":  "OracleSet",
			"Account":          acct.ClassicAddress,
			"OracleDocumentID": docID,
			"Provider":         randHexBytes(r, 8),
			"AssetClass":       randHexBytes(r, 8),
			"LastUpdateTime":   uint32(time.Now().Unix()),
			"PriceDataSeries": []any{
				map[string]any{
					"PriceData": map[string]any{
						"BaseAsset":  "XRP",
						"QuoteAsset": "USD",
						"AssetPrice": fmt.Sprintf("%X", price),
						"Scale":      uint32(r.IntN(6)),
					},
				},
			},
		},
		Secret: acct.Seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "OracleSet",
		RequiresAll:     []string{"PriceOracle"},
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.OracleSet(r) },
	})
}
