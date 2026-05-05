package accounts

import (
	"math/rand/v2"
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// stubSubmit captures every SubmitTxJSON call to verify intent.
type stubSubmit struct {
	calls []map[string]any
}

func (s *stubSubmit) SubmitTxJSON(secret string, tx map[string]any) (*rpcclient.SubmitResult, error) {
	s.calls = append(s.calls, tx)
	return &rpcclient.SubmitResult{EngineResult: "tesSUCCESS"}, nil
}

func (s *stubSubmit) AccountInfo(addr string) (*rpcclient.AccountInfoResult, error) {
	return &rpcclient.AccountInfoResult{
		Account:  addr,
		Balance:  "10000000000", // 10000 XRP, well above reserve
		Sequence: 1,
	}, nil
}

func TestAssignTiers_SpreadsAcrossTiers(t *testing.T) {
	pool, err := NewPool(42, 20)
	if err != nil {
		t.Fatal(err)
	}
	weights := TierWeights{
		Rich:       10,
		AtReserve:  4,
		Multisig:   3,
		RegularKey: 2,
		Blackholed: 1,
	}
	rng := rand.New(rand.NewPCG(42, 0))
	AssignTiers(pool, weights, rng)

	counts := map[Tier]int{}
	for _, w := range pool.All() {
		counts[w.Tier]++
	}
	if counts[Rich]+counts[AtReserve]+counts[Multisig]+counts[RegularKey]+counts[Blackholed] != 20 {
		t.Errorf("counts = %+v, want sum 20", counts)
	}
	if counts[Rich] < 5 {
		t.Errorf("Rich count = %d, want >= 5 (weight 10/20)", counts[Rich])
	}
	if counts[Blackholed] < 1 {
		t.Errorf("Blackholed count = %d, want >= 1", counts[Blackholed])
	}
}

func TestPickTier_ReturnsCorrectTier(t *testing.T) {
	pool, err := NewPool(42, 10)
	if err != nil {
		t.Fatal(err)
	}
	weights := TierWeights{Rich: 5, AtReserve: 3, Multisig: 2}
	rng := rand.New(rand.NewPCG(42, 0))
	AssignTiers(pool, weights, rng)

	for i := 0; i < 20; i++ {
		w := pool.PickTier(Multisig, rng)
		if w == nil {
			t.Fatal("PickTier(Multisig) returned nil")
		}
		if w.Tier != Multisig {
			t.Errorf("Tier = %v, want Multisig", w.Tier)
		}
	}
}

func TestPickTier_NilWhenEmpty(t *testing.T) {
	pool, err := NewPool(42, 5)
	if err != nil {
		t.Fatal(err)
	}
	weights := TierWeights{Rich: 5}
	rng := rand.New(rand.NewPCG(42, 0))
	AssignTiers(pool, weights, rng)
	if w := pool.PickTier(Blackholed, rng); w != nil {
		t.Errorf("PickTier(Blackholed) = %+v, want nil (no blackholed wallets)", w)
	}
}

func TestSetupAtReserve_SubmitsPaymentToTreasury(t *testing.T) {
	w := &Wallet{Index: 0, ClassicAddress: "rTest1", Seed: "sTest1", Tier: AtReserve}
	stub := &stubSubmit{}
	if err := setupAtReserve(stub, w); err != nil {
		t.Fatal(err)
	}
	if len(stub.calls) != 1 {
		t.Fatalf("submit calls = %d, want 1", len(stub.calls))
	}
	tx := stub.calls[0]
	if tx["TransactionType"] != "Payment" {
		t.Errorf("tx_type = %v, want Payment", tx["TransactionType"])
	}
	if tx["Destination"] != GenesisAddress {
		t.Errorf("destination = %v, want %s", tx["Destination"], GenesisAddress)
	}
}
