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
		counts[tx.TransactionType]++
	}
	for _, k := range []string{"Payment", "TrustSet", "OfferCreate"} {
		if counts[k] == 0 {
			t.Fatalf("tx type %q never selected in 200 picks", k)
		}
	}
}

func TestPickTx_SkipsUnsatisfiedAmendments(t *testing.T) {
	registerForTest(CandidateTx{
		TransactionType: "FakeAMMDeposit",
		RequiresAll:     []string{"AMM"},
		Build: func(_ *Generator, _ anyRand) (*Tx, error) {
			t := &Tx{TransactionType: "FakeAMMDeposit"}
			return t, nil
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
		if tx.TransactionType == "FakeAMMDeposit" {
			t.Fatal("selected tx type whose required amendment was not enabled")
		}
	}
}
