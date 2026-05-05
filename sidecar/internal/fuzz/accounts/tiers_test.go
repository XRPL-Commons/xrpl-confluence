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

func TestSetupMultisig_InstallsSignerList(t *testing.T) {
	w := &Wallet{Index: 0, ClassicAddress: "rTest", Seed: "sTest", Tier: Multisig}
	signers := []*Wallet{
		{Index: 1, ClassicAddress: "rA", Seed: "sA"},
		{Index: 2, ClassicAddress: "rB", Seed: "sB"},
		{Index: 3, ClassicAddress: "rC", Seed: "sC"},
	}
	stub := &stubSubmit{}
	if err := setupMultisig(stub, w, signers); err != nil {
		t.Fatal(err)
	}
	if len(stub.calls) != 1 {
		t.Fatalf("submit calls = %d, want 1", len(stub.calls))
	}
	tx := stub.calls[0]
	if tx["TransactionType"] != "SignerListSet" {
		t.Errorf("tx_type = %v, want SignerListSet", tx["TransactionType"])
	}
	if tx["SignerQuorum"] != uint32(2) {
		t.Errorf("quorum = %v, want 2", tx["SignerQuorum"])
	}
	entries, ok := tx["SignerEntries"].([]map[string]any)
	if !ok || len(entries) != 3 {
		t.Errorf("entries = %+v, want 3", tx["SignerEntries"])
	}
}

func TestSetupRegularKey_SetsKeyThenDisablesMaster(t *testing.T) {
	w := &Wallet{Index: 0, ClassicAddress: "rTest", Seed: "sTest", Tier: RegularKey}
	stub := &stubSubmit{}
	if err := setupRegularKey(stub, w, "rRegKey"); err != nil {
		t.Fatal(err)
	}
	if len(stub.calls) != 2 {
		t.Fatalf("submit calls = %d, want 2 (SetRegularKey + AccountSet)", len(stub.calls))
	}
	if stub.calls[0]["TransactionType"] != "SetRegularKey" {
		t.Errorf("call[0] = %v, want SetRegularKey", stub.calls[0]["TransactionType"])
	}
	if stub.calls[0]["RegularKey"] != "rRegKey" {
		t.Errorf("RegularKey = %v, want rRegKey", stub.calls[0]["RegularKey"])
	}
	if stub.calls[1]["TransactionType"] != "AccountSet" {
		t.Errorf("call[1] = %v, want AccountSet", stub.calls[1]["TransactionType"])
	}
	if stub.calls[1]["SetFlag"] != uint32(4) {
		t.Errorf("SetFlag = %v, want 4 (asfDisableMaster)", stub.calls[1]["SetFlag"])
	}
}

func TestSetupBlackholed_DisablesMaster(t *testing.T) {
	w := &Wallet{Index: 0, ClassicAddress: "rTest", Seed: "sTest", Tier: Blackholed}
	stub := &stubSubmit{}
	if err := setupBlackholed(stub, w); err != nil {
		t.Fatal(err)
	}
	if len(stub.calls) != 1 {
		t.Fatalf("submit calls = %d, want 1", len(stub.calls))
	}
	if stub.calls[0]["TransactionType"] != "AccountSet" {
		t.Errorf("tx_type = %v", stub.calls[0]["TransactionType"])
	}
	if stub.calls[0]["SetFlag"] != uint32(4) {
		t.Errorf("SetFlag = %v, want 4 (asfDisableMaster)", stub.calls[0]["SetFlag"])
	}
}

func TestApplyAll_RoutesByTier(t *testing.T) {
	pool, err := NewPool(42, 10)
	if err != nil {
		t.Fatal(err)
	}
	weights := TierWeights{Rich: 4, AtReserve: 2, Multisig: 2, RegularKey: 1, Blackholed: 1}
	rng := rand.New(rand.NewPCG(42, 0))
	AssignTiers(pool, weights, rng)
	stub := &stubSubmit{}
	if err := ApplyAll(stub, pool); err != nil {
		t.Fatal(err)
	}
	// Rich = 0 calls. AtReserve = 1 call. Multisig = 1. RegularKey = 2. Blackholed = 1.
	// Total: 0*4 + 1*2 + 1*2 + 2*1 + 1*1 = 7.
	if len(stub.calls) != 7 {
		t.Fatalf("submit calls = %d, want 7", len(stub.calls))
	}
}
