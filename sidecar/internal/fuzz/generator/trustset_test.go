package generator

import (
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/accounts"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
)

func TestTrustSet_WellFormed(t *testing.T) {
	pool, _ := accounts.NewPool(0xc3, 5)
	rng := corpus.NewRNG(123)
	g := New(pool)

	tx, err := g.TrustSet(rng.Rand())
	if err != nil {
		t.Fatal(err)
	}
	if tx.TransactionType != "TrustSet" {
		t.Fatalf("type = %q", tx.TransactionType)
	}
	if tx.LimitAmount == nil {
		t.Fatal("LimitAmount missing")
	}
	for _, k := range []string{"currency", "issuer", "value"} {
		if _, ok := tx.LimitAmount[k]; !ok {
			t.Fatalf("LimitAmount missing key %q", k)
		}
	}
	if tx.LimitAmount["issuer"] == tx.Account {
		t.Fatal("cannot trust-set to self")
	}
}
