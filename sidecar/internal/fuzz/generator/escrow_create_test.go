package generator

import (
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/accounts"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
)

func TestEscrowCreate_WellFormed(t *testing.T) {
	pool, _ := accounts.NewPool(0xe5c, 4)
	g := New(pool)
	r := corpus.NewRNG(1).Rand()

	tx, err := g.EscrowCreate(r)
	if err != nil {
		t.Fatal(err)
	}
	if tx.TransactionType() != "EscrowCreate" {
		t.Fatalf("type = %q", tx.TransactionType())
	}
	if _, ok := tx.Fields["Account"].(string); !ok {
		t.Fatal("Account missing")
	}
	if _, ok := tx.Fields["Destination"].(string); !ok {
		t.Fatal("Destination missing")
	}
	if tx.Fields["Destination"] == tx.Fields["Account"] {
		t.Fatal("Destination equals Account")
	}
	if _, ok := tx.Fields["Amount"].(string); !ok {
		t.Fatal("Amount missing or not drops string")
	}
	_, fa := tx.Fields["FinishAfter"].(uint32)
	_, ca := tx.Fields["CancelAfter"].(uint32)
	if !fa && !ca {
		t.Fatal("need at least one of FinishAfter / CancelAfter")
	}
}
