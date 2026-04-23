package generator

import (
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/accounts"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
)

func TestPickTx_OnlyProducesTypesWithSatisfiedAmendments(t *testing.T) {
	pool, _ := accounts.NewPool(1, 5)
	g := New(pool)
	r := corpus.NewRNG(1).Rand()

	// Empty amendment set → only txs requiring no amendments are eligible.
	// M1's three types (Payment, TrustSet, OfferCreate) all pre-date amendments,
	// so all should be selectable.
	counts := map[string]int{}
	for i := 0; i < 200; i++ {
		tx, err := g.PickTx(r, []string{})
		if err != nil {
			t.Fatal(err)
		}
		counts[tx.TransactionType()]++
	}
	for _, k := range []string{"Payment", "TrustSet", "OfferCreate"} {
		if counts[k] == 0 {
			t.Fatalf("tx type %q never selected in 200 picks", k)
		}
	}
}

func TestPickTx_DeterministicFromSeed(t *testing.T) {
	pool, _ := accounts.NewPool(0xfeed, 5)
	g := New(pool)

	r1 := corpus.NewRNG(42).Rand()
	r2 := corpus.NewRNG(42).Rand()

	var seq1, seq2 []string
	for i := 0; i < 50; i++ {
		a, _ := g.PickTx(r1, []string{})
		b, _ := g.PickTx(r2, []string{})
		seq1 = append(seq1, a.TransactionType())
		seq2 = append(seq2, b.TransactionType())
	}
	for i := range seq1 {
		if seq1[i] != seq2[i] {
			t.Fatalf("diverged at step %d: %q vs %q", i, seq1[i], seq2[i])
		}
	}
}

func TestPickTx_SkipsUnsatisfiedAmendments(t *testing.T) {
	registerForTest(CandidateTx{
		TransactionType: "FakeAMMDeposit",
		RequiresAll:     []string{"AMM"},
		Build: func(_ *Generator, _ anyRand) (*Tx, error) {
			return &Tx{Fields: map[string]any{"TransactionType": "FakeAMMDeposit"}}, nil
		},
	})
	t.Cleanup(unregisterForTest("FakeAMMDeposit"))

	pool, _ := accounts.NewPool(1, 5)
	g := New(pool)
	r := corpus.NewRNG(1).Rand()

	for i := 0; i < 100; i++ {
		tx, err := g.PickTx(r, []string{}) // AMM NOT enabled
		if err != nil {
			t.Fatal(err)
		}
		if tx.TransactionType() == "FakeAMMDeposit" {
			t.Fatal("selected tx type whose required amendment was not enabled")
		}
	}
}
