package accounts

import (
	"math/rand/v2"
	"testing"
)

func TestAssignTiers_SpreadsAcrossTiers(t *testing.T) {
	pool, err := NewPool(42, 20)
	if err != nil {
		t.Fatal(err)
	}
	weights := TierWeights{
		Rich:       10,
		AtReserve:  4,
		Multisig:   3,
		RegularKey: 2,
		Blackholed: 1,
	}
	rng := rand.New(rand.NewPCG(42, 0))
	AssignTiers(pool, weights, rng)

	counts := map[Tier]int{}
	for _, w := range pool.All() {
		counts[w.Tier]++
	}
	if counts[Rich]+counts[AtReserve]+counts[Multisig]+counts[RegularKey]+counts[Blackholed] != 20 {
		t.Errorf("counts = %+v, want sum 20", counts)
	}
	if counts[Rich] < 5 {
		t.Errorf("Rich count = %d, want >= 5 (weight 10/20)", counts[Rich])
	}
	if counts[Blackholed] < 1 {
		t.Errorf("Blackholed count = %d, want >= 1", counts[Blackholed])
	}
}

func TestPickTier_ReturnsCorrectTier(t *testing.T) {
	pool, err := NewPool(42, 10)
	if err != nil {
		t.Fatal(err)
	}
	weights := TierWeights{Rich: 5, AtReserve: 3, Multisig: 2}
	rng := rand.New(rand.NewPCG(42, 0))
	AssignTiers(pool, weights, rng)

	for i := 0; i < 20; i++ {
		w := pool.PickTier(Multisig, rng)
		if w == nil {
			t.Fatal("PickTier(Multisig) returned nil")
		}
		if w.Tier != Multisig {
			t.Errorf("Tier = %v, want Multisig", w.Tier)
		}
	}
}

func TestPickTier_NilWhenEmpty(t *testing.T) {
	pool, err := NewPool(42, 5)
	if err != nil {
		t.Fatal(err)
	}
	weights := TierWeights{Rich: 5}
	rng := rand.New(rand.NewPCG(42, 0))
	AssignTiers(pool, weights, rng)
	if w := pool.PickTier(Blackholed, rng); w != nil {
		t.Errorf("PickTier(Blackholed) = %+v, want nil (no blackholed wallets)", w)
	}
}
