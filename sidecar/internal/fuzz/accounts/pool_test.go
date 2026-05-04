package accounts

import (
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
)

func TestNewPool_DeterministicFromSeed(t *testing.T) {
	p1, err := NewPool(0xcafef00d, 10)
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	p2, err := NewPool(0xcafef00d, 10)
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	for i := 0; i < 10; i++ {
		if p1.All()[i].ClassicAddress != p2.All()[i].ClassicAddress {
			t.Fatalf("pool diverged at i=%d: %q vs %q",
				i, p1.All()[i].ClassicAddress, p2.All()[i].ClassicAddress)
		}
	}
}

func TestNewPool_HonorsSize(t *testing.T) {
	p, _ := NewPool(1, 7)
	if got := len(p.All()); got != 7 {
		t.Fatalf("size = %d, want 7", got)
	}
}

func TestPool_PickReturnsInPool(t *testing.T) {
	p, _ := NewPool(1, 5)
	rng := corpus.NewRNG(99)
	seen := map[string]bool{}
	for _, w := range p.All() {
		seen[w.ClassicAddress] = true
	}
	for i := 0; i < 50; i++ {
		w := p.Pick(rng.Rand())
		if !seen[w.ClassicAddress] {
			t.Fatalf("Pick returned address not in pool: %q", w.ClassicAddress)
		}
	}
}

func TestPool_PickTwoDistinct(t *testing.T) {
	p, _ := NewPool(1, 5)
	rng := corpus.NewRNG(99)
	for i := 0; i < 50; i++ {
		a, b := p.PickTwoDistinct(rng.Rand())
		if a.ClassicAddress == b.ClassicAddress {
			t.Fatalf("PickTwoDistinct returned same address %q", a.ClassicAddress)
		}
	}
}

func TestPool_PickTwoDistinct_PanicsWhenTooSmall(t *testing.T) {
	p, _ := NewPool(1, 1)
	rng := corpus.NewRNG(1)
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for pool of size 1")
		}
	}()
	p.PickTwoDistinct(rng.Rand())
}

func TestRotateTiers_RecyclesXRP(t *testing.T) {
	t.Skip("M1 ships rich-only; rotation is a no-op stub here. " +
		"Replace this skip when M2/M3 add multiple tiers.")
}
