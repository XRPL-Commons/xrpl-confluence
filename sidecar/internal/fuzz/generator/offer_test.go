package generator

import (
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/accounts"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
)

func TestOfferCreate_WellFormed(t *testing.T) {
	pool, _ := accounts.NewPool(0xd4, 5)
	rng := corpus.NewRNG(1)
	g := New(pool)

	tx, err := g.OfferCreate(rng.Rand())
	if err != nil {
		t.Fatal(err)
	}
	if tx.TransactionType != "OfferCreate" {
		t.Fatalf("type = %q", tx.TransactionType)
	}
	if _, ok := tx.TakerPays.(string); !ok {
		t.Fatalf("TakerPays = %T, want string", tx.TakerPays)
	}
	gets, ok := tx.TakerGets.(map[string]any)
	if !ok {
		t.Fatalf("TakerGets = %T, want map", tx.TakerGets)
	}
	if gets["issuer"] == tx.Account {
		t.Fatal("issuer equals Account")
	}
}
