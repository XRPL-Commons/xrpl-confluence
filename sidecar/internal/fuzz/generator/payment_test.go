package generator

import (
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/accounts"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
)

func TestPayment_ValidXRPPayment(t *testing.T) {
	pool, _ := accounts.NewPool(0xa1, 5)
	rng := corpus.NewRNG(123)
	g := New(pool)

	tx, err := g.Payment(rng.Rand())
	if err != nil {
		t.Fatalf("Payment: %v", err)
	}
	if tx.TransactionType != "Payment" {
		t.Fatalf("TransactionType = %q, want Payment", tx.TransactionType)
	}
	if tx.Account == "" || tx.Destination == "" {
		t.Fatal("missing Account or Destination")
	}
	if tx.Account == tx.Destination {
		t.Fatal("Account and Destination must differ")
	}
	amt, ok := tx.Amount.(string)
	if !ok {
		t.Fatalf("Amount = %T, want string (drops)", tx.Amount)
	}
	if amt == "" {
		t.Fatal("empty amount")
	}
}

func TestPayment_DeterministicFromSeed(t *testing.T) {
	pool, _ := accounts.NewPool(0xb2, 5)
	g := New(pool)

	r1 := corpus.NewRNG(7).Rand()
	r2 := corpus.NewRNG(7).Rand()
	a, _ := g.Payment(r1)
	b, _ := g.Payment(r2)
	if a.Account != b.Account || a.Destination != b.Destination || a.Amount != b.Amount {
		t.Fatalf("Payment not deterministic: %+v vs %+v", a, b)
	}
}
