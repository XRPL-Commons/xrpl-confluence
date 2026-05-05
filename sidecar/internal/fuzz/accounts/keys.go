// Package accounts owns the fuzzer's account pool and deterministic keypair
// derivation. All keys are derived from (fuzzSeed, index) via SHA-256 so a
// run is fully reproducible from its logged seed.
package accounts

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	addresscodec "github.com/Peersyst/xrpl-go/address-codec"
	"github.com/Peersyst/xrpl-go/pkg/crypto"
	"github.com/Peersyst/xrpl-go/xrpl/wallet"
)

// Wallet is the fuzzer's account representation: address + secret seed + its
// deterministic index in the pool.
type Wallet struct {
	Index          int
	ClassicAddress string
	Seed           string // XRPL "s..." secret, suitable for sign-and-submit RPC
	Tier           Tier
}

// DeriveWallet produces a reproducible XRPL wallet for (fuzzSeed, index).
//
// Derivation: SHA-256(bigEndian(fuzzSeed) || bigEndian(index)) → first 16
// bytes → XRPL ed25519 family seed → wallet.FromSeed. The ed25519 path is
// chosen because it is simpler (one-way hash from seed bytes, no secp256k1
// family-generator iteration), which makes determinism easy to reason about.
//
// Adaptation note: xrpl-go v0.1.18 does not expose wallet.FromEntropy.
// Instead, addresscodec.EncodeSeed converts raw entropy bytes to an XRPL
// base58-encoded seed string ("s..."), which wallet.FromSeed then consumes
// via the standard derivation path.
func DeriveWallet(fuzzSeed uint64, index int) (*Wallet, error) {
	var buf [16]byte
	binary.BigEndian.PutUint64(buf[0:8], fuzzSeed)
	binary.BigEndian.PutUint64(buf[8:16], uint64(index))
	sum := sha256.Sum256(buf[:])
	entropy := sum[:16] // XRPL family seeds are 16 bytes of entropy

	// Encode the raw entropy as an XRPL ed25519 seed string ("s...").
	xrplSeed, err := addresscodec.EncodeSeed(entropy, crypto.ED25519())
	if err != nil {
		return nil, fmt.Errorf("addresscodec.EncodeSeed: %w", err)
	}

	w, err := wallet.FromSeed(xrplSeed, "")
	if err != nil {
		return nil, fmt.Errorf("wallet.FromSeed: %w", err)
	}

	return &Wallet{
		Index:          index,
		ClassicAddress: w.ClassicAddress.String(),
		Seed:           w.Seed,
	}, nil
}
