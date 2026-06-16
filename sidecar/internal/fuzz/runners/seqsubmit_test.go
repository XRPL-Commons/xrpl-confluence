package runners

import (
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/generator"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// fakeSeqClient is a programmable seqClient: it returns a fixed account
// sequence from AccountInfoAt and replays a scripted list of engine results
// from SubmitTxBlob, capturing the Sequence stamped on each signed tx.
type fakeSeqClient struct {
	accountSeq    int
	results       []string
	idx           int
	signedSeqs    []int
	acctInfoCalls int
}

func (f *fakeSeqClient) SignLocal(secret string, tx map[string]any) (string, error) {
	if s, ok := tx["Sequence"].(int); ok {
		f.signedSeqs = append(f.signedSeqs, s)
	}
	return "blob", nil
}

func (f *fakeSeqClient) SubmitTxBlob(blob string) (*rpcclient.SubmitResult, error) {
	r := "tesSUCCESS"
	if f.idx < len(f.results) {
		r = f.results[f.idx]
	}
	f.idx++
	return &rpcclient.SubmitResult{EngineResult: r}, nil
}

func (f *fakeSeqClient) AccountInfoAt(account, ledgerIndex string) (*rpcclient.AccountInfoResult, error) {
	f.acctInfoCalls++
	return &rpcclient.AccountInfoResult{Account: account, Sequence: f.accountSeq}, nil
}

func (f *fakeSeqClient) LedgerCurrentIndex() (uint32, error) { return 100, nil }

func seqTestTx(account string) *generator.Tx {
	return &generator.Tx{
		Fields: map[string]any{"Account": account, "TransactionType": "Payment"},
		Secret: "s",
	}
}

func TestSeqManager_AdvancesOnConsumingResults(t *testing.T) {
	// tesSUCCESS, tec*, and terQUEUED all consume the sequence, so the local
	// counter advances 5 -> 6 -> 7 and account_info is queried exactly once.
	f := &fakeSeqClient{accountSeq: 5, results: []string{"tesSUCCESS", "tecUNFUNDED_PAYMENT", "terQUEUED"}}
	sm := newSeqManager(f, nil)
	for i := 0; i < 3; i++ {
		if _, err := sm.submitTx(seqTestTx("rA")); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}
	if got := f.signedSeqs; len(got) != 3 || got[0] != 5 || got[1] != 6 || got[2] != 7 {
		t.Fatalf("signed sequences = %v, want [5 6 7]", got)
	}
	if f.acctInfoCalls != 1 {
		t.Fatalf("account_info calls = %d, want 1 (primed once, then cached)", f.acctInfoCalls)
	}
}

func TestSeqManager_HoldsSequenceOnNonConsumingFailure(t *testing.T) {
	// tem* does not consume the sequence; the next tx for the account must
	// reuse it so no gap is left behind.
	f := &fakeSeqClient{accountSeq: 10, results: []string{"temMALFORMED", "tesSUCCESS"}}
	sm := newSeqManager(f, nil)
	if _, err := sm.submitTx(seqTestTx("rB")); err != nil {
		t.Fatal(err)
	}
	if _, err := sm.submitTx(seqTestTx("rB")); err != nil {
		t.Fatal(err)
	}
	if got := f.signedSeqs; len(got) != 2 || got[0] != 10 || got[1] != 10 {
		t.Fatalf("signed sequences = %v, want [10 10] (held after non-consuming failure)", got)
	}
}

func TestSeqManager_ResyncsOnSequenceDrift(t *testing.T) {
	// tefPAST_SEQ invalidates the cached sequence; the next peek re-fetches the
	// authoritative value from the open ledger.
	f := &fakeSeqClient{accountSeq: 20, results: []string{"tefPAST_SEQ", "tesSUCCESS"}}
	sm := newSeqManager(f, nil)
	if _, err := sm.submitTx(seqTestTx("rC")); err != nil {
		t.Fatal(err)
	}
	f.accountSeq = 25 // ledger has moved on; resync must pick this up
	if _, err := sm.submitTx(seqTestTx("rC")); err != nil {
		t.Fatal(err)
	}
	if got := f.signedSeqs; len(got) != 2 || got[0] != 20 || got[1] != 25 {
		t.Fatalf("signed sequences = %v, want [20 25] (resynced from ledger)", got)
	}
	if f.acctInfoCalls != 2 {
		t.Fatalf("account_info calls = %d, want 2 (initial + resync)", f.acctInfoCalls)
	}
}

func TestConsumesSequence(t *testing.T) {
	consuming := []string{"tesSUCCESS", "terQUEUED", "tecUNFUNDED_PAYMENT", "tecPATH_DRY"}
	for _, r := range consuming {
		if !consumesSequence(r) {
			t.Errorf("consumesSequence(%q) = false, want true", r)
		}
	}
	nonConsuming := []string{"temMALFORMED", "tefPAST_SEQ", "telCAN_NOT_QUEUE_FULL", "terPRE_SEQ"}
	for _, r := range nonConsuming {
		if consumesSequence(r) {
			t.Errorf("consumesSequence(%q) = true, want false", r)
		}
	}
}
