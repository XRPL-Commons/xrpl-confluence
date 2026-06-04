package generator

import (
	"encoding/binary"
	"encoding/hex"
	mathrand "math/rand/v2"
	"strings"
)

// Shared helpers for the transaction builders.

// NFToken transaction flags (subset the generator uses).
const (
	tfTransferable uint32 = 0x00000008 // NFTokenMint: token may be transferred
	tfSellNFToken  uint32 = 0x00000001 // NFTokenCreateOffer: offer is a sell offer
)

// seedFor returns the secret seed of the pool wallet with the given classic
// address. Reference tx types (OfferCancel, CheckCancel, …) use it to sign as
// the object's owner.
func (g *Generator) seedFor(address string) (string, bool) {
	for _, w := range g.pool.All() {
		if w.ClassicAddress == address {
			return w.Seed, true
		}
	}
	return "", false
}

// randHexBytes returns n random bytes as an uppercase hex string — the form
// XRPL uses for blob fields (CredentialType, URI, Provider, metadata, …).
func randHexBytes(r *mathrand.Rand, n int) string {
	buf := make([]byte, n)
	for i := 0; i < len(buf); i += 8 {
		var word [8]byte
		binary.BigEndian.PutUint64(word[:], r.Uint64())
		copy(buf[i:], word[:])
	}
	return strings.ToUpper(hex.EncodeToString(buf))
}
