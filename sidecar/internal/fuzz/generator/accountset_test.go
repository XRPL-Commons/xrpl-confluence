package generator

import (
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/accounts"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
)

func TestAccountSet_WellFormed(t *testing.T) {
	pool, _ := accounts.NewPool(0xe5, 3)
	g := New(pool)
	rng := corpus.NewRNG(1).Rand()

	tx, err := g.AccountSet(rng)
	if err != nil {
		t.Fatal(err)
	}
	if tx.TransactionType() != "AccountSet" {
		t.Fatalf("type = %q", tx.TransactionType())
	}
	if _, ok := tx.Fields["Account"].(string); !ok {
		t.Fatal("Account missing")
	}
	_, setOK := tx.Fields["SetFlag"].(uint32)
	_, clearOK := tx.Fields["ClearFlag"].(uint32)
	if !setOK && !clearOK {
		t.Fatal("neither SetFlag nor ClearFlag present")
	}
}
