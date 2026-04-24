package generator

import (
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/accounts"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
)

func TestEscrowFinish_RequiresTrackedEscrow(t *testing.T) {
	pool, _ := accounts.NewPool(0xe5f, 4)
	g := New(pool)
	r := corpus.NewRNG(1).Rand()

	_, err := g.EscrowFinish(r)
	if err == nil {
		t.Fatal("expected error with empty tracker")
	}

	g.Tracker().Escrows().Record(pool.All()[0].ClassicAddress, 10)
	tx, err := g.EscrowFinish(r)
	if err != nil {
		t.Fatal(err)
	}
	if tx.TransactionType() != "EscrowFinish" {
		t.Fatalf("type = %q", tx.TransactionType())
	}
	if tx.Fields["Owner"] != pool.All()[0].ClassicAddress {
		t.Fatalf("Owner = %v", tx.Fields["Owner"])
	}
	if tx.Fields["OfferSequence"].(uint32) != 10 {
		t.Fatalf("OfferSequence = %v", tx.Fields["OfferSequence"])
	}
}

func TestEscrowFinish_CanBuildGate(t *testing.T) {
	pool, _ := accounts.NewPool(1, 4)
	g := New(pool)
	r := corpus.NewRNG(1).Rand()

	for i := 0; i < 200; i++ {
		tx, err := g.PickTx(r, []string{})
		if err != nil {
			t.Fatal(err)
		}
		if tx.TransactionType() == "EscrowFinish" {
			t.Fatal("EscrowFinish selected despite empty tracker")
		}
	}
}
