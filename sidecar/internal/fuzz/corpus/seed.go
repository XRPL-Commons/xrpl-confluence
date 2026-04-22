// Package corpus owns deterministic seeding and divergence recording for the
// fuzzer. Every stochastic decision in the fuzzer MUST flow through an RNG
// created here; otherwise runs cannot be reproduced from a logged seed.
package corpus

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"fmt"
	mathrand "math/rand/v2"
	"os"
	"strconv"
)

// RNG wraps a PCG-seeded rand.Rand and preserves the original seed for
// logging and corpus entries.
type RNG struct {
	seed uint64
	r    *mathrand.Rand
}

// NewRNG returns a deterministic RNG. The second PCG stream identifier is
// derived from the seed so callers do not need to plumb two values.
func NewRNG(seed uint64) *RNG {
	const streamTweak uint64 = 0x9e3779b97f4a7c15 // golden-ratio constant
	src := mathrand.NewPCG(seed, seed^streamTweak)
	return &RNG{seed: seed, r: mathrand.New(src)}
}

// Seed returns the seed passed to NewRNG.
func (r *RNG) Seed() uint64 { return r.seed }

// Rand returns the underlying *math/rand/v2.Rand for use by callers.
func (r *RNG) Rand() *mathrand.Rand { return r.r }

// SeedFromEnv returns the uint64 in envVar if it parses, else a fresh
// crypto-random uint64. Use this at process startup so every run has a
// recorded seed, even when none was supplied.
func SeedFromEnv(envVar string) uint64 {
	if v := os.Getenv(envVar); v != "" {
		if s, err := strconv.ParseUint(v, 10, 64); err == nil {
			return s
		}
	}
	var b [8]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("crypto/rand.Read: %v", err))
	}
	return binary.BigEndian.Uint64(b[:])
}
