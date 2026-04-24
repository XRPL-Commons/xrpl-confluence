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
	if tx.TransactionType() != "OfferCreate" {
		t.Fatalf("type = %q", tx.TransactionType())
	}
	if _, ok := tx.Fields["TakerPays"].(string); !ok {
		t.Fatalf("TakerPays = %T, want string", tx.Fields["TakerPays"])
	}
	gets, ok := tx.Fields["TakerGets"].(map[string]any)
	if !ok {
		t.Fatalf("TakerGets = %T, want map", tx.Fields["TakerGets"])
	}
	if gets["issuer"] == tx.Fields["Account"] {
		t.Fatal("issuer equals Account")
	}
}
