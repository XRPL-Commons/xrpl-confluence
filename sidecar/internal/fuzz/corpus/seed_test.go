package corpus

import (
	"os"
	"testing"
)

func TestRNG_SameSeedProducesSameStream(t *testing.T) {
	a := NewRNG(42)
	b := NewRNG(42)
	for i := 0; i < 100; i++ {
		if a.Rand().Uint64() != b.Rand().Uint64() {
			t.Fatalf("streams diverged at i=%d", i)
		}
	}
}

func TestRNG_DifferentSeedsProduceDifferentStream(t *testing.T) {
	a := NewRNG(1)
	b := NewRNG(2)
	same := true
	for i := 0; i < 100 && same; i++ {
		if a.Rand().Uint64() != b.Rand().Uint64() {
			same = false
		}
	}
	if same {
		t.Fatal("different seeds produced identical stream")
	}
}

func TestRNG_SeedAccessorReturnsOriginal(t *testing.T) {
	r := NewRNG(0xdeadbeef)
	if r.Seed() != 0xdeadbeef {
		t.Fatalf("Seed() = %#x, want 0xdeadbeef", r.Seed())
	}
}

func TestSeedFromEnv_ParsesEnv(t *testing.T) {
	t.Setenv("FUZZ_SEED", "12345")
	if got := SeedFromEnv("FUZZ_SEED"); got != 12345 {
		t.Fatalf("got %d, want 12345", got)
	}
}

func TestSeedFromEnv_RandomFallback(t *testing.T) {
	_ = os.Unsetenv("FUZZ_SEED")
	a := SeedFromEnv("FUZZ_SEED")
	b := SeedFromEnv("FUZZ_SEED")
	if a == b {
		t.Fatal("two crypto-random fallbacks returned identical seeds (astronomically unlikely)")
	}
}
