package runners

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// Both the topology RPC and the mainnet RPC are stubbed. Verifies ReplayRun
// walks the range, rewrites txs, submits them, and closes cleanly.
func TestReplay_RunWalksRangeAndSubmits(t *testing.T) {
	var submits atomic.Int64
	topoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			_, _ = w.Write([]byte(`{"result":{"engine_result":"tesSUCCESS","engine_result_code":0,"engine_result_message":"","tx_json":{"hash":"H","Sequence":` + strconv.Itoa(int(n)) + `},"status":"success"}}`))
		case "tx":
			_, _ = w.Write([]byte(`{"result":{"meta":{"TransactionResult":"tesSUCCESS","AffectedNodes":[]},"validated":true}}`))
		case "account_info":
			_, _ = w.Write([]byte(`{"result":{"account_data":{"Account":"r","Balance":"1000000000","Sequence":1},"status":"success"}}`))
		default:
			_, _ = w.Write([]byte(`{"result":{"status":"success"}}`))
		}
	})
	srvA := httptest.NewServer(topoHandler)
	defer srvA.Close()
	srvB := httptest.NewServer(topoHandler)
	defer srvB.Close()

	mainnetHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":{"ledger":{"transactions":[
			{"TransactionType":"Payment","Account":"rA","Destination":"rB","Amount":"1000"},
			{"TransactionType":"Payment","Account":"rC","Destination":"rA","Amount":"2000"}
		]},"status":"success"}}`))
	})
	mainnetSrv := httptest.NewServer(mainnetHandler)
	defer mainnetSrv.Close()

	cfg := ReplayConfig{
		NodeURLs:    []string{srvA.URL, srvB.URL},
		SubmitURL:   srvA.URL,
		MainnetURL:  mainnetSrv.URL,
		Seed:        0x9,
		AccountN:    4,
		LedgerStart: 100,
		LedgerEnd:   102,
		CorpusDir:   t.TempDir(),
		BatchClose:  10 * time.Millisecond,
		SkipFund:    true,
		SkipSetup:   true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stats, err := ReplayRun(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TxsSubmitted != 6 { // 2 txs × 3 ledgers
		t.Fatalf("TxsSubmitted = %d, want 6", stats.TxsSubmitted)
	}
	if stats.Divergences != 0 {
		t.Fatalf("Divergences = %d, want 0", stats.Divergences)
	}
}
