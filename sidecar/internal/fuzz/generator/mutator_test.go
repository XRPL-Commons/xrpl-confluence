package generator

import (
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
)

func txForMut() *Tx {
	return &Tx{
		Fields: map[string]any{
			"TransactionType": "Payment",
			"Account":         "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh",
			"Destination":     "rEXAMPLEEEEEEEEEEEEEEEEEEEEEEEEEEEEE",
			"Amount":          "1000000",
		},
		Secret: "snoSECRET",
	}
}

func TestMutator_ApplyDeleteField(t *testing.T) {
	m := NewMutator()
	r := corpus.NewRNG(1).Rand()
	tx := txForMut()
	orig := len(tx.Fields)

	got, applied := m.apply(r, tx, mutDeleteField{})
	if !applied {
		t.Fatal("expected mutDeleteField to apply")
	}
	if len(got.Fields) != orig-1 {
		t.Fatalf("got %d fields, want %d", len(got.Fields), orig-1)
	}
	// TransactionType must survive (it's required).
	if got.Fields["TransactionType"] != "Payment" {
		t.Fatal("TransactionType was deleted")
	}
}

func TestMutator_ApplyCorruptDropsAmount(t *testing.T) {
	m := NewMutator()
	r := corpus.NewRNG(1).Rand()
	tx := txForMut()

	got, applied := m.apply(r, tx, mutCorruptDrops{})
	if !applied {
		t.Fatal("expected mutCorruptDrops to apply")
	}
	amt, ok := got.Fields["Amount"].(string)
	if !ok {
		t.Fatalf("Amount missing or wrong type: %T", got.Fields["Amount"])
	}
	if amt == "1000000" {
		t.Fatal("Amount was not corrupted")
	}
}

func TestMutator_CorruptAccount(t *testing.T) {
	m := NewMutator()
	r := corpus.NewRNG(1).Rand()
	tx := txForMut()

	got, applied := m.apply(r, tx, mutCorruptAddress{})
	if !applied {
		t.Fatal("expected mutCorruptAddress to apply")
	}
	// At least one address field must differ from the original.
	anyMutated := false
	for _, f := range addrFieldNames {
		orig, hasOrig := tx.Fields[f].(string)
		newV, hasNew := got.Fields[f].(string)
		if hasOrig && hasNew && orig != newV {
			anyMutated = true
			break
		}
		if hasOrig && !hasNew {
			anyMutated = true
			break
		}
	}
	if !anyMutated {
		t.Fatal("no address field was mutated")
	}
}

func TestMutator_NegateUint32(t *testing.T) {
	m := NewMutator()
	r := corpus.NewRNG(1).Rand()
	tx := &Tx{
		Fields: map[string]any{
			"TransactionType": "AccountSet",
			"Account":         "rX",
			"SetFlag":         uint32(5),
		},
		Secret: "s",
	}
	got, applied := m.apply(r, tx, mutOverflowUint32{})
	if !applied {
		t.Fatal("expected mutOverflowUint32 to apply")
	}
	if got.Fields["SetFlag"] == uint32(5) {
		t.Fatal("SetFlag was not mutated")
	}
}

func TestMutator_ZeroUint32(t *testing.T) {
	m := NewMutator()
	r := corpus.NewRNG(1).Rand()
	tx := &Tx{
		Fields: map[string]any{
			"TransactionType": "TicketCreate",
			"Account":         "rX",
			"TicketCount":     uint32(3),
		},
		Secret: "s",
	}
	got, applied := m.apply(r, tx, mutZeroUint32{})
	if !applied {
		t.Fatal("expected mutZeroUint32 to apply")
	}
	if v, _ := got.Fields["TicketCount"].(uint32); v != 0 {
		t.Fatalf("TicketCount = %v, want 0", got.Fields["TicketCount"])
	}
}

func TestMutator_MaybeAppliesAtGivenRate(t *testing.T) {
	m := NewMutator()
	r := corpus.NewRNG(1).Rand()

	// rate=1.0 → always mutates; rate=0.0 → never.
	for i := 0; i < 20; i++ {
		got, mutated := m.Maybe(r, txForMut(), 1.0)
		if !mutated || got == nil {
			t.Fatal("rate=1.0 should always mutate")
		}
	}
	for i := 0; i < 20; i++ {
		_, mutated := m.Maybe(r, txForMut(), 0.0)
		if mutated {
			t.Fatal("rate=0.0 should never mutate")
		}
	}
}

func TestMutator_DeterministicFromSeed(t *testing.T) {
	m := NewMutator()
	r1 := corpus.NewRNG(42).Rand()
	r2 := corpus.NewRNG(42).Rand()

	var seq1, seq2 []string
	for i := 0; i < 20; i++ {
		a, _ := m.Maybe(r1, txForMut(), 1.0)
		b, _ := m.Maybe(r2, txForMut(), 1.0)
		// Canonicalise to a string that captures which field changed.
		seq1 = append(seq1, fingerprintTx(a))
		seq2 = append(seq2, fingerprintTx(b))
	}
	for i := range seq1 {
		if seq1[i] != seq2[i] {
			t.Fatalf("mutation diverged at i=%d: %q vs %q", i, seq1[i], seq2[i])
		}
	}
}

// fingerprintTx is a stable string representation of Fields keyed+valued,
// sorted by key. Helps assert deterministic mutation across runs.
func fingerprintTx(tx *Tx) string {
	keys := make([]string, 0, len(tx.Fields))
	for k := range tx.Fields {
		keys = append(keys, k)
	}
	// stable order
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	out := ""
	for _, k := range keys {
		out += k + "="
		switch v := tx.Fields[k].(type) {
		case string:
			out += v
		default:
			out += ""
		}
		out += ";"
	}
	return out
}
