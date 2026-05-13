package runners

import (
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/metrics"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

func TestClassifyFailure_PrefersRPCErrorOverEngineResult(t *testing.T) {
	res := &rpcclient.SubmitResult{EngineResult: "tesSUCCESS"}
	got := classifyFailure(res, errors.New("rpc error highFee (11): Fee of 12000"))
	if got != "highFee" {
		t.Fatalf("classifyFailure with rpc error = %q, want highFee", got)
	}
}

func TestClassifyFailure_FallsBackToRPCErrorClass(t *testing.T) {
	got := classifyFailure(nil, errors.New("connection refused"))
	if got != "rpc_error" {
		t.Fatalf("classifyFailure = %q, want rpc_error", got)
	}
}

func TestClassifyFailure_UsesEngineResultWhenNoError(t *testing.T) {
	res := &rpcclient.SubmitResult{EngineResult: "tecUNFUNDED_OFFER"}
	got := classifyFailure(res, nil)
	if got != "tecUNFUNDED_OFFER" {
		t.Fatalf("classifyFailure = %q, want tecUNFUNDED_OFFER", got)
	}
}

func TestClassifyFailure_HandlesNilRes(t *testing.T) {
	got := classifyFailure(nil, nil)
	if got != "unknown" {
		t.Fatalf("classifyFailure(nil,nil) = %q, want unknown", got)
	}
}

func TestRecordFailure_IncrementsMetricAndWritesRunLog(t *testing.T) {
	dir := t.TempDir()
	rl, err := corpus.NewRunLog(dir, 0xdead)
	if err != nil {
		t.Fatal(err)
	}
	defer rl.Close()

	m := metrics.New()
	var seq int64

	recordFailure(m, rl, 1, &seq, 7, "OfferCreate",
		map[string]any{"Account": "rA", "TakerPays": "1"},
		"sEd1",
		&rpcclient.SubmitResult{EngineResult: "tecUNFUNDED_OFFER", EngineResultMessage: "no funds"},
		nil,
	)

	entries, err := corpus.ReadRunLog(filepath.Join(dir, "runs", "dead.ndjson"))
	if err != nil {
		t.Fatalf("ReadRunLog: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 run-log entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Step != 7 || e.TxType != "OfferCreate" || e.Result != "tecUNFUNDED_OFFER" {
		t.Fatalf("run-log entry mismatch: %+v", e)
	}
	if e.TxHash != "" {
		t.Fatalf("failed entry should have empty TxHash, got %q", e.TxHash)
	}

	body := scrapeMetrics(t, m)
	if !strings.Contains(body, `fuzz_txs_failed_total{result="tecUNFUNDED_OFFER",tx_type="OfferCreate"} 1`) {
		t.Fatalf("fuzz_txs_failed_total not incremented; body:\n%s", body)
	}
}

func TestRecordFailure_NilMetricsAndLogTolerated(t *testing.T) {
	recordFailure(nil, nil, 0, nil, 0, "Payment", nil, "", nil, errors.New("boom"))
}

func TestRecordFailure_AccumulatesAcrossCalls(t *testing.T) {
	m := metrics.New()
	var seq int64
	for i := 0; i < 20; i++ {
		recordFailure(m, nil, 3, &seq, i, "Payment", nil, "", nil,
			errors.New("rpc error notReady (13)"))
	}
	body := scrapeMetrics(t, m)
	if !strings.Contains(body, `fuzz_txs_failed_total{result="notReady",tx_type="Payment"} 20`) {
		t.Fatalf("expected 20 notReady failures; body:\n%s", body)
	}
}

func scrapeMetrics(t *testing.T, m *metrics.Registry) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	m.Handler().ServeHTTP(rec, req)
	return rec.Body.String()
}
