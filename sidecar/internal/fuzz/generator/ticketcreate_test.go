package generator

import (
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/accounts"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
)

func TestTicketCreate_WellFormed(t *testing.T) {
	pool, _ := accounts.NewPool(0x77, 3)
	g := New(pool)
	rng := corpus.NewRNG(1).Rand()

	for i := 0; i < 20; i++ {
		tx, err := g.TicketCreate(rng)
		if err != nil {
			t.Fatal(err)
		}
		if tx.TransactionType() != "TicketCreate" {
			t.Fatalf("type = %q", tx.TransactionType())
		}
		cnt, ok := tx.Fields["TicketCount"].(uint32)
		if !ok {
			t.Fatalf("TicketCount missing or wrong type: %T", tx.Fields["TicketCount"])
		}
		if cnt < 1 || cnt > 250 {
			t.Fatalf("TicketCount = %d, out of 1..250 range", cnt)
		}
	}
}
