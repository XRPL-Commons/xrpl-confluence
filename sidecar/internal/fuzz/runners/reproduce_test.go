package runners

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestReproduce_ReplaysRunLog(t *testing.T) {
	var submits atomic.Int64
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct{ Method string }
		_ = json.Unmarshal(body, &req)
		switch req.Method {
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
	srvA := httptest.NewServer(handler)
	defer srvA.Close()
	srvB := httptest.NewServer(handler)
	defer srvB.Close()

	dir := t.TempDir()
	logPath := filepath.Join(dir, "reproducer.ndjson")
	lines := []string{
		`{"step":0,"tx_type":"Payment","fields":{"TransactionType":"Payment","Account":"rA","Destination":"rB","Amount":"1"},"secret":"s1"}`,
		`{"step":1,"tx_type":"Payment","fields":{"TransactionType":"Payment","Account":"rC","Destination":"rA","Amount":"2"},"secret":"s2"}`,
		`{"step":2,"tx_type":"Payment","fields":{"TransactionType":"Payment","Account":"rB","Destination":"rC","Amount":"3"},"secret":"s3"}`,
	}
	if err := os.WriteFile(logPath, []byte(lines[0]+"\n"+lines[1]+"\n"+lines[2]+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := ReproduceConfig{
		NodeURLs:  []string{srvA.URL, srvB.URL},
		SubmitURL: srvA.URL,
		LogPath:   logPath,
		CorpusDir: dir,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stats, err := Reproduce(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TxsSubmitted != 3 {
		t.Fatalf("TxsSubmitted = %d, want 3", stats.TxsSubmitted)
	}
}
