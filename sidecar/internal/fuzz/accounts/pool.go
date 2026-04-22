package accounts

import (
	"fmt"
	mathrand "math/rand/v2"
)

// Pool is a deterministic set of funded accounts. M1 ships a single rich
// tier; future milestones add at-reserve / under-reserve / multisig /
// regular-key / blackholed tiers behind the same interface.
type Pool struct {
	wallets []*Wallet
}

// NewPool derives `size` deterministic wallets from fuzzSeed. The pool is
// un-funded at this point; see FundFromGenesis.
func NewPool(fuzzSeed uint64, size int) (*Pool, error) {
	if size < 1 {
		return nil, fmt.Errorf("pool size must be >= 1, got %d", size)
	}
	ws := make([]*Wallet, size)
	for i := 0; i < size; i++ {
		w, err := DeriveWallet(fuzzSeed, i)
		if err != nil {
			return nil, fmt.Errorf("derive wallet %d: %w", i, err)
		}
		ws[i] = w
	}
	return &Pool{wallets: ws}, nil
}

// All returns the backing slice. Callers must not mutate it.
func (p *Pool) All() []*Wallet { return p.wallets }

// Pick returns a uniformly random wallet from the pool.
func (p *Pool) Pick(r *mathrand.Rand) *Wallet {
	return p.wallets[r.IntN(len(p.wallets))]
}

// PickTwoDistinct returns two wallets guaranteed to differ. Panics if the
// pool has fewer than 2 entries (caller error — a single-element pool cannot
// satisfy the contract).
func (p *Pool) PickTwoDistinct(r *mathrand.Rand) (*Wallet, *Wallet) {
	if len(p.wallets) < 2 {
		panic("PickTwoDistinct requires pool of size >= 2")
	}
	a := p.Pick(r)
	for {
		b := p.Pick(r)
		if b.ClassicAddress != a.ClassicAddress {
			return a, b
		}
	}
}
