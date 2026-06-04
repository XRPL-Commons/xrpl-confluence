package generator

import (
	mathrand "math/rand/v2"
)

// SignerListSet installs a multi-sign list of 1..3 other pool accounts with a
// quorum that is satisfiable by the assigned weights. MultiSign is a retired
// (always-on) amendment, so this needs no gating.
func (g *Generator) SignerListSet(r *mathrand.Rand) (*Tx, error) {
	acct := g.pool.Pick(r)

	n := r.IntN(3) + 1 // 1..3 signers
	entries := make([]any, 0, n)
	used := map[string]struct{}{acct.ClassicAddress: {}}
	var quorum uint32
	for len(entries) < n {
		signer := g.pool.Pick(r)
		if _, dup := used[signer.ClassicAddress]; dup {
			continue
		}
		used[signer.ClassicAddress] = struct{}{}
		weight := uint32(r.IntN(3) + 1) // 1..3
		quorum += weight
		entries = append(entries, map[string]any{
			"SignerEntry": map[string]any{
				"Account":      signer.ClassicAddress,
				"SignerWeight": weight,
			},
		})
	}

	return &Tx{
		Fields: map[string]any{
			"TransactionType": "SignerListSet",
			"Account":         acct.ClassicAddress,
			"SignerQuorum":    quorum,
			"SignerEntries":   entries,
		},
		Secret: acct.Seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "SignerListSet",
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.SignerListSet(r) },
	})
}
