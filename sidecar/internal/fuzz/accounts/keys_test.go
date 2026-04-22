package accounts

import (
	"testing"

	"github.com/Peersyst/xrpl-go/xrpl/wallet"
)

func TestDeriveWallet_DeterministicFromSeed(t *testing.T) {
	w1, err := DeriveWallet(0xdeadbeef, 0)
	if err != nil {
		t.Fatalf("DeriveWallet: %v", err)
	}
	w2, err := DeriveWallet(0xdeadbeef, 0)
	if err != nil {
		t.Fatalf("DeriveWallet: %v", err)
	}
	if w1.ClassicAddress != w2.ClassicAddress {
		t.Fatalf("same (seed, index) produced different addresses: %q vs %q",
			w1.ClassicAddress, w2.ClassicAddress)
	}
}

func TestDeriveWallet_IndexChangesAddress(t *testing.T) {
	w0, _ := DeriveWallet(0xdeadbeef, 0)
	w1, _ := DeriveWallet(0xdeadbeef, 1)
	if w0.ClassicAddress == w1.ClassicAddress {
		t.Fatal("index 0 and 1 produced identical addresses")
	}
}

func TestDeriveWallet_SeedChangesAddress(t *testing.T) {
	wa, _ := DeriveWallet(1, 0)
	wb, _ := DeriveWallet(2, 0)
	if wa.ClassicAddress == wb.ClassicAddress {
		t.Fatal("seeds 1 and 2 produced identical addresses at index 0")
	}
}

func TestDeriveWallet_ReturnsUsableWallet(t *testing.T) {
	w, err := DeriveWallet(0x1234, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Round-trip through wallet.FromSeed using the returned secret.
	_, err = wallet.FromSeed(w.Seed, "")
	if err != nil {
		t.Fatalf("derived seed is not a valid XRPL seed: %v", err)
	}
}
