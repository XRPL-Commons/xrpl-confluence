package runners

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Stubs every RPC path the runner touches (feature, submit, server_info,
// ledger, tx) with constant responses so the runner's control flow is
// exercised without a real network.
func TestRealtime_RunSubmitsAndClosesCorpus(t *testing.T) {
	var submits atomic.Int64
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct{ Method string }
		_ = json.Unmarshal(body, &req)

		switch req.Method {
		case "feature":
			_, _ = w.Write([]byte(`{"result":{"features":{},"status":"success"}}`))
		case "server_info":
			_, _ = w.Write([]byte(`{"result":{"info":{"server_state":"full","peers":1,"validated_ledger":{"seq":100,"hash":"AAA"}}}}`))
		case "ledger":
			_, _ = w.Write([]byte(`{"result":{"ledger":{"ledger_index":"100","ledger_hash":"AAA","account_hash":"BBB","transaction_hash":"CCC"},"status":"success"}}`))
		case "submit":
			n := submits.Add(1)
			hash := "H" + strings.Repeat("0", 63-len("H")) + string(rune('0'+int(n%10)))
			_, _ = w.Write([]byte(`{"result":{"engine_result":"tesSUCCESS","engine_result_code":0,"engine_result_message":"","tx_json":{"hash":"` + hash + `"},"status":"success"}}`))
		case "tx":
			_, _ = w.Write([]byte(`{"result":{"meta":{"TransactionResult":"tesSUCCESS"},"validated":true}}`))
		default:
			_, _ = w.Write([]byte(`{"result":{"status":"success"}}`))
		}
	})
	srvA := httptest.NewServer(handler)
	defer srvA.Close()
	srvB := httptest.NewServer(handler)
	defer srvB.Close()

	corpusDir := t.TempDir()

	cfg := Config{
		NodeURLs:   []string{srvA.URL, srvB.URL},
		SubmitURL:  srvA.URL,
		Seed:       0x1234,
		AccountN:   4,
		TxCount:    5,
		CorpusDir:  corpusDir,
		BatchClose: 50 * time.Millisecond,
		SkipFund:   true, // Tests don't model genesis state; skip the funding phase.
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stats, err := Run(ctx, cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.TxsSubmitted != 5 {
		t.Fatalf("TxsSubmitted = %d, want 5", stats.TxsSubmitted)
	}
	// No divergences in this run (both nodes returned identical hashes).
	if stats.Divergences != 0 {
		t.Fatalf("Divergences = %d, want 0", stats.Divergences)
	}
	// Corpus dir exists but contains no divergence files.
	entries, _ := os.ReadDir(filepath.Join(corpusDir, "divergences"))
	if len(entries) != 0 {
		t.Fatalf("corpus had %d entries, want 0", len(entries))
	}
}
