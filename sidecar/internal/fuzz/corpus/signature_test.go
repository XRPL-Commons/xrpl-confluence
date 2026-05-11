package corpus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeDivergenceFile(t *testing.T, dir string, d *Divergence) string {
	t.Helper()
	path := filepath.Join(dir, "div.json")
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDivergenceSignature_TxResult(t *testing.T) {
	dir := t.TempDir()
	path := writeDivergenceFile(t, dir, &Divergence{
		Kind: "tx_result",
		Details: map[string]any{
			"tx_hash": "ABC",
			"tx_type": "Payment",
		},
	})
	sig, err := LoadDivergenceSignature(path)
	if err != nil {
		t.Fatal(err)
	}
	if sig.Kind != "tx_result" || sig.TxType != "Payment" {
		t.Fatalf("got %+v, want kind=tx_result tx_type=Payment", sig)
	}
}

func TestLoadDivergenceSignature_StateHash(t *testing.T) {
	dir := t.TempDir()
	path := writeDivergenceFile(t, dir, &Divergence{
		Kind: "state_hash",
		Details: map[string]any{
			"comparison": map[string]any{
				"divergences": []any{
					map[string]any{"field": "account_hash"},
				},
			},
		},
	})
	sig, err := LoadDivergenceSignature(path)
	if err != nil {
		t.Fatal(err)
	}
	if sig.Kind != "state_hash" || sig.Field != "account_hash" {
		t.Fatalf("got %+v, want kind=state_hash field=account_hash", sig)
	}
}

func TestLoadDivergenceSignature_Invariant(t *testing.T) {
	dir := t.TempDir()
	path := writeDivergenceFile(t, dir, &Divergence{
		Kind: "invariant",
		Details: map[string]any{
			"invariant": "pool_balance_monotone",
		},
	})
	sig, err := LoadDivergenceSignature(path)
	if err != nil {
		t.Fatal(err)
	}
	if sig.Kind != "invariant" || sig.Invariant != "pool_balance_monotone" {
		t.Fatalf("got %+v, want kind=invariant invariant=pool_balance_monotone", sig)
	}
}

func TestLoadDivergenceSignature_MissingKind(t *testing.T) {
	dir := t.TempDir()
	path := writeDivergenceFile(t, dir, &Divergence{Details: map[string]any{}})
	if _, err := LoadDivergenceSignature(path); err == nil {
		t.Fatal("expected error for missing kind")
	}
}

func TestSignature_MatchesKindAndTxType(t *testing.T) {
	sig := DivergenceSignature{Kind: "metadata", TxType: "OfferCreate"}
	if !sig.Matches(&Divergence{Kind: "metadata", Details: map[string]any{"tx_type": "OfferCreate"}}) {
		t.Fatal("expected match")
	}
	if sig.Matches(&Divergence{Kind: "metadata", Details: map[string]any{"tx_type": "Payment"}}) {
		t.Fatal("expected no match (different tx_type)")
	}
	if sig.Matches(&Divergence{Kind: "tx_result", Details: map[string]any{"tx_type": "OfferCreate"}}) {
		t.Fatal("expected no match (different kind)")
	}
}

func TestSignature_MatchesStateHashField(t *testing.T) {
	sig := DivergenceSignature{Kind: "state_hash", Field: "account_hash"}
	d := &Divergence{
		Kind: "state_hash",
		Details: map[string]any{
			"comparison": map[string]any{
				"divergences": []any{
					map[string]any{"field": "account_hash"},
				},
			},
		},
	}
	if !sig.Matches(d) {
		t.Fatal("expected match on account_hash field")
	}
	d.Details["comparison"].(map[string]any)["divergences"] = []any{
		map[string]any{"field": "ledger_hash"},
	}
	if sig.Matches(d) {
		t.Fatal("expected no match (different field)")
	}
}

func TestSignature_WildcardSubfield(t *testing.T) {
	// Empty TxType on the signature means: match any tx_type for this Kind.
	sig := DivergenceSignature{Kind: "tx_result"}
	if !sig.Matches(&Divergence{Kind: "tx_result", Details: map[string]any{"tx_type": "Payment"}}) {
		t.Fatal("empty TxType should match any tx_result")
	}
}

func TestSignature_NilDivergence(t *testing.T) {
	sig := DivergenceSignature{Kind: "tx_result"}
	if sig.Matches(nil) {
		t.Fatal("nil divergence must not match")
	}
}

func TestSignature_FromInMemoryDivergence(t *testing.T) {
	cases := []struct {
		name string
		d    Divergence
		want string // expected Key()
	}{
		{"tx_result", Divergence{Kind: "tx_result", Details: map[string]any{"tx_type": "Payment"}}, "tx_result_Payment"},
		{"metadata", Divergence{Kind: "metadata", Details: map[string]any{"tx_type": "OfferCreate"}}, "metadata_OfferCreate"},
		{"invariant", Divergence{Kind: "invariant", Details: map[string]any{"invariant": "pool_balance_monotone"}}, "invariant_pool_balance_monotone"},
		{"crash", Divergence{Kind: "crash"}, "crash"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Signature(&c.d).Key()
			if got != c.want {
				t.Errorf("Key()=%q want %q", got, c.want)
			}
		})
	}
}

func TestSignature_KeySanitisesUnsafeCharacters(t *testing.T) {
	sig := DivergenceSignature{Kind: "invariant", Invariant: "weird/path:name"}
	got := sig.Key()
	want := "invariant_weird_path_name"
	if got != want {
		t.Errorf("Key()=%q want %q", got, want)
	}
}

func TestSignature_NilSafe(t *testing.T) {
	if got := Signature(nil); got.Kind != "" {
		t.Errorf("Signature(nil) = %+v, want zero", got)
	}
}
