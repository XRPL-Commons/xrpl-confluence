package generator

import (
	mathrand "math/rand/v2"
)

// MPTokenIssuanceCreate flags (issuance capabilities) and MPTokenIssuanceSet
// flags (lock/unlock) the generator uses.
const (
	tfMPTCanLock     uint32 = 0x00000002
	tfMPTCanTransfer uint32 = 0x00000020
	tfMPTLock        uint32 = 0x00000001
	tfMPTUnlock      uint32 = 0x00000002
)

// MPTokenIssuanceCreate creates a lockable, transferable MPToken issuance. The
// runner derives the MPTokenIssuanceID from (issuer, sequence) so
// MPTokenAuthorize / MPTokenIssuanceSet / MPTokenIssuanceDestroy can reference
// it. Gated by the MPTokensV1 amendment.
func (g *Generator) MPTokenIssuanceCreate(r *mathrand.Rand) (*Tx, error) {
	acct := g.pool.Pick(r)
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "MPTokenIssuanceCreate",
			"Account":         acct.ClassicAddress,
			"AssetScale":      uint32(r.IntN(6)),
			"Flags":           tfMPTCanLock | tfMPTCanTransfer,
		},
		Secret: acct.Seed,
	}, nil
}

func init() {
	Register(CandidateTx{
		TransactionType: "MPTokenIssuanceCreate",
		RequiresAll:     []string{"MPTokensV1"},
		Build:           func(g *Generator, r anyRand) (*Tx, error) { return g.MPTokenIssuanceCreate(r) },
	})
}
