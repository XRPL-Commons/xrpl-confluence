package accounts

import (
	"testing"

	"github.com/Peersyst/xrpl-go/xrpl/wallet"
)

// TestXRPLGo_WalletFromSeed confirms the library can build a deterministic
// wallet from an XRPL secret string. This is a smoke test for our dep pin —
// if it ever breaks after a version bump, we catch it here.
func TestXRPLGo_WalletFromSeed(t *testing.T) {
	// Well-known XRPL test seed (genesis-like).
	const seed = "snoPBrXtMeMyMHUVTgbuqAfg1SUTb"
	w, err := wallet.FromSeed(seed, "")
	if err != nil {
		t.Fatalf("wallet.FromSeed: %v", err)
	}
	if w.ClassicAddress == "" {
		t.Fatal("empty classic address")
	}
	// Genesis seed → known address.
	const wantAddr = "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"
	if w.ClassicAddress.String() != wantAddr {
		t.Fatalf("addr = %q, want %q", w.ClassicAddress, wantAddr)
	}
}
