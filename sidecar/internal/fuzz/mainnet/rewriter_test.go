package mainnet

import (
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/accounts"
)

func TestRewriter_ReplacesKnownAddressFields(t *testing.T) {
	pool, _ := accounts.NewPool(0xabc, 4)
	rw := NewRewriter(pool)

	tx := map[string]any{
		"TransactionType": "Payment",
		"Account":         "rSomeMainnetAccount123",
		"Destination":     "rSomeOtherMainnetAccount456",
		"Amount":          "1000000",
		"Sequence":        float64(100),
		"SigningPubKey":   "ABCDEF",
		"TxnSignature":    "deadbeef",
		"hash":            "HASH",
	}

	out, ok, _ := rw.Rewrite(tx)
	if !ok {
		t.Fatal("expected rewrite to succeed")
	}
	if out["Account"] == "rSomeMainnetAccount123" {
		t.Fatal("Account not rewritten")
	}
	if out["Destination"] == "rSomeOtherMainnetAccount456" {
		t.Fatal("Destination not rewritten")
	}
	for _, stripped := range []string{"Sequence", "SigningPubKey", "TxnSignature", "hash"} {
		if _, exists := out[stripped]; exists {
			t.Fatalf("%q should have been stripped", stripped)
		}
	}
	if out["Amount"] != "1000000" {
		t.Fatalf("Amount = %v", out["Amount"])
	}
	if out["TransactionType"] != "Payment" {
		t.Fatalf("TransactionType = %v", out["TransactionType"])
	}
}

func TestRewriter_IOUIssuerRewritten(t *testing.T) {
	pool, _ := accounts.NewPool(0xabc, 4)
	rw := NewRewriter(pool)

	tx := map[string]any{
		"TransactionType": "TrustSet",
		"Account":         "rHolder",
		"LimitAmount": map[string]any{
			"currency": "USD",
			"issuer":   "rIssuer",
			"value":    "100",
		},
	}
	out, ok, _ := rw.Rewrite(tx)
	if !ok {
		t.Fatal("expected rewrite")
	}
	limit := out["LimitAmount"].(map[string]any)
	if limit["issuer"] == "rIssuer" {
		t.Fatal("issuer not rewritten")
	}
	if limit["currency"] != "USD" || limit["value"] != "100" {
		t.Fatalf("non-address fields corrupted: %v", limit)
	}
}

func TestRewriter_SameMainnetAddrMapsToSamePoolAddr(t *testing.T) {
	pool, _ := accounts.NewPool(0xabc, 4)
	rw := NewRewriter(pool)

	tx1 := map[string]any{"TransactionType": "Payment", "Account": "rSame", "Destination": "rOther"}
	tx2 := map[string]any{"TransactionType": "Payment", "Account": "rSame", "Destination": "rThird"}

	out1, _, _ := rw.Rewrite(tx1)
	out2, _, _ := rw.Rewrite(tx2)

	if out1["Account"] != out2["Account"] {
		t.Fatalf("same mainnet Account mapped differently: %v vs %v",
			out1["Account"], out2["Account"])
	}
}

func TestRewriter_RejectsCollapsedIssuerAccount(t *testing.T) {
	// Pool of size 1 → every mainnet address maps to the same pool account.
	pool, _ := accounts.NewPool(0xabc, 1)
	rw := NewRewriter(pool)

	tx := map[string]any{
		"TransactionType": "TrustSet",
		"Account":         "rHolder",
		"LimitAmount": map[string]any{
			"currency": "USD",
			"issuer":   "rIssuer",
			"value":    "100",
		},
	}
	_, ok, reason := rw.Rewrite(tx)
	if ok {
		t.Fatal("expected rejection for collapsed issuer/account")
	}
	if reason == "" {
		t.Fatal("expected a non-empty rejection reason")
	}
}

func TestRewriter_SecretFor(t *testing.T) {
	pool, _ := accounts.NewPool(0xabc, 4)
	rw := NewRewriter(pool)
	tx := map[string]any{"TransactionType": "Payment", "Account": "rAnything", "Destination": "rElse"}
	out, _, _ := rw.Rewrite(tx)
	account := out["Account"].(string)
	secret, ok := rw.SecretFor(account)
	if !ok {
		t.Fatal("SecretFor: no seed for rewritten account")
	}
	if secret == "" {
		t.Fatal("empty secret")
	}
}
