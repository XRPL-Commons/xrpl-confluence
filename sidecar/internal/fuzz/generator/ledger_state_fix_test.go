package generator

import (
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/accounts"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
)

func TestLedgerStateFix_WellFormed(t *testing.T) {
	pool, _ := accounts.NewPool(0xffee, 4)
	g := New(pool)

	tx, err := g.LedgerStateFix(corpus.NewRNG(5).Rand())
	if err != nil {
		t.Fatal(err)
	}
	if tx.TransactionType() != "LedgerStateFix" {
		t.Fatalf("type = %q", tx.TransactionType())
	}
	if got := tx.Fields["LedgerFixType"].(uint32); got != 1 {
		t.Fatalf("LedgerFixType = %d, want 1", got)
	}
	owner, _ := tx.Fields["Owner"].(string)
	if owner == "" {
		t.Fatal("Owner is required for the NFTokenPageLink fix")
	}
	if tx.Fields["Account"] == owner {
		t.Fatal("Account and Owner should differ")
	}
}

// LedgerStateFix must not be selected unless fixNFTokenPageLinks is enabled.
func TestLedgerStateFix_GatedByAmendment(t *testing.T) {
	pool, _ := accounts.NewPool(1, 4)
	g := New(pool)
	r := corpus.NewRNG(1).Rand()
	for i := 0; i < 500; i++ {
		tx, err := g.PickTx(r, []string{})
		if err != nil {
			t.Fatal(err)
		}
		if tx.TransactionType() == "LedgerStateFix" {
			t.Fatal("LedgerStateFix selected without fixNFTokenPageLinks enabled")
		}
	}
}
