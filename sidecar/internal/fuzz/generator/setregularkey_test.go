package generator

import (
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/accounts"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
)

func TestSetRegularKey_WellFormed(t *testing.T) {
	pool, _ := accounts.NewPool(0xf6, 3)
	g := New(pool)
	rng := corpus.NewRNG(1).Rand()

	set, cleared := 0, 0
	for i := 0; i < 100; i++ {
		tx, err := g.SetRegularKey(rng)
		if err != nil {
			t.Fatal(err)
		}
		if tx.TransactionType() != "SetRegularKey" {
			t.Fatalf("type = %q", tx.TransactionType())
		}
		if _, ok := tx.Fields["Account"].(string); !ok {
			t.Fatal("Account missing")
		}
		if regKey, ok := tx.Fields["RegularKey"]; ok {
			s, ok := regKey.(string)
			if !ok || s == "" {
				t.Fatal("RegularKey present but empty or wrong type")
			}
			if s == tx.Fields["Account"].(string) {
				t.Fatal("RegularKey equal to Account")
			}
			set++
		} else {
			cleared++
		}
	}
	if set == 0 || cleared == 0 {
		t.Fatalf("expected both paths: set=%d cleared=%d", set, cleared)
	}
}
