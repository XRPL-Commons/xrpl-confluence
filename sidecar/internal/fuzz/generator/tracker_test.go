package generator

import (
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
)

func TestEscrowTracker_EmptyInitially(t *testing.T) {
	tr := NewTracker()
	if n := tr.Escrows().Count(); n != 0 {
		t.Fatalf("Count = %d, want 0", n)
	}
	if _, _, ok := tr.Escrows().PickOpen(corpus.NewRNG(1).Rand()); ok {
		t.Fatal("PickOpen on empty tracker should return ok=false")
	}
}

func TestEscrowTracker_RecordThenPick(t *testing.T) {
	tr := NewTracker()
	tr.Escrows().Record("rOwner1", 10)
	tr.Escrows().Record("rOwner2", 20)

	if tr.Escrows().Count() != 2 {
		t.Fatalf("Count = %d, want 2", tr.Escrows().Count())
	}

	r := corpus.NewRNG(1).Rand()
	owner, seq, ok := tr.Escrows().PickOpen(r)
	if !ok {
		t.Fatal("expected a pick")
	}
	if (owner != "rOwner1" || seq != 10) && (owner != "rOwner2" || seq != 20) {
		t.Fatalf("got (%q,%d), want one of the recorded pairs", owner, seq)
	}
}

func TestEscrowTracker_DeterministicFromSeed(t *testing.T) {
	tr := NewTracker()
	for i := 0; i < 5; i++ {
		tr.Escrows().Record("rO", uint32(i))
	}

	r1 := corpus.NewRNG(42).Rand()
	r2 := corpus.NewRNG(42).Rand()
	var s1, s2 []uint32
	for i := 0; i < 20; i++ {
		_, x, _ := tr.Escrows().PickOpen(r1)
		_, y, _ := tr.Escrows().PickOpen(r2)
		s1 = append(s1, x)
		s2 = append(s2, y)
	}
	for i := range s1 {
		if s1[i] != s2[i] {
			t.Fatalf("diverged at i=%d: %d vs %d", i, s1[i], s2[i])
		}
	}
}
