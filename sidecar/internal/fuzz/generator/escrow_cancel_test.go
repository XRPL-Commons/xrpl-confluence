package generator

import (
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/accounts"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
)

func TestEscrowCancel_RequiresTrackedEscrow(t *testing.T) {
	pool, _ := accounts.NewPool(0xe5c, 4)
	g := New(pool)
	r := corpus.NewRNG(1).Rand()

	_, err := g.EscrowCancel(r)
	if err == nil {
		t.Fatal("expected error with empty tracker")
	}

	g.Tracker().Escrows().Record(pool.All()[0].ClassicAddress, 99)
	tx, err := g.EscrowCancel(r)
	if err != nil {
		t.Fatal(err)
	}
	if tx.TransactionType() != "EscrowCancel" {
		t.Fatalf("type = %q", tx.TransactionType())
	}
	if tx.Fields["Owner"] != pool.All()[0].ClassicAddress {
		t.Fatalf("Owner = %v", tx.Fields["Owner"])
	}
	if tx.Fields["OfferSequence"].(uint32) != 99 {
		t.Fatalf("OfferSequence = %v", tx.Fields["OfferSequence"])
	}
}
