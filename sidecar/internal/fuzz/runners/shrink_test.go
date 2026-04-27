package runners

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
)

// stubShrinkHandler emulates one node of a 2-node topology where node B
// reports a different TransactionResult on a specific submit step — so a prefix
// that includes that step reproduces a layer-2 divergence.
//
// node:               which node we're emulating ("A" or "B").
// divergeOnSubmit:    submit number (1-indexed) at which node B should report
//                     a different result. Node A always agrees.
// submits:            shared per-server submit counter.
func stubShrinkHandler(node string, divergeOnSubmit int64, submits *atomic.Int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Method string           `json:"method"`
			Params []map[string]any `json:"params"`
		}
		_ = json.Unmarshal(body, &req)
		switch req.Method {
		case "server_info":
			_, _ = w.Write([]byte(`{"result":{"info":{"server_state":"full","peers":1,"validated_ledger":{"seq":100,"hash":"AAA"}}}}`))
		case "ledger":
			_, _ = w.Write([]byte(`{"result":{"ledger":{"ledger_index":"100","ledger_hash":"AAA","account_hash":"BBB","transaction_hash":"CCC"},"status":"success"}}`))
		case "submit":
			n := submits.Add(1)
			body := fmt.Sprintf(`{"result":{"engine_result":"tesSUCCESS","engine_result_code":0,"engine_result_message":"","tx_json":{"hash":"H%d","Sequence":%d},"status":"success"}}`, n, n)
			_, _ = w.Write([]byte(body))
		case "tx":
			hash, _ := req.Params[0]["transaction"].(string)
			step, _ := strconv.Atoi(hash[1:]) // "H3" → 3
			result := "tesSUCCESS"
			if node == "B" && int64(step) == divergeOnSubmit {
				result = "tecPATH_DRY"
			}
			body := fmt.Sprintf(`{"result":{"meta":{"TransactionResult":"%s","AffectedNodes":[]},"validated":true}}`, result)
			_, _ = w.Write([]byte(body))
		default:
			_, _ = w.Write([]byte(`{"result":{"status":"success"}}`))
		}
	}
}

func writeShrinkInputs(t *testing.T, dir string, lines []string, sig corpus.Divergence) (logPath, divPath string) {
	t.Helper()
	logPath = filepath.Join(dir, "in.ndjson")
	out := ""
	for _, l := range lines {
		out += l + "\n"
	}
	if err := os.WriteFile(logPath, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	divPath = filepath.Join(dir, "div.json")
	b, _ := json.Marshal(sig)
	if err := os.WriteFile(divPath, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return
}

func TestShrink_MatchedAtPrefix(t *testing.T) {
	var submitsA, submitsB atomic.Int64
	srvA := httptest.NewServer(stubShrinkHandler("A", 0, &submitsA))
	defer srvA.Close()
	srvB := httptest.NewServer(stubShrinkHandler("B", 2, &submitsB)) // diverges on submit #2
	defer srvB.Close()

	dir := t.TempDir()
	logPath, divPath := writeShrinkInputs(t, dir,
		[]string{
			`{"step":0,"tx_type":"Payment","fields":{"TransactionType":"Payment","Account":"rA","Destination":"rB","Amount":"1"},"secret":"s"}`,
			`{"step":1,"tx_type":"Payment","fields":{"TransactionType":"Payment","Account":"rC","Destination":"rA","Amount":"2"},"secret":"s"}`,
			`{"step":2,"tx_type":"Payment","fields":{"TransactionType":"Payment","Account":"rB","Destination":"rC","Amount":"3"},"secret":"s"}`,
		},
		corpus.Divergence{Kind: "tx_result", Details: map[string]any{"tx_type": "Payment"}},
	)

	cfg := ShrinkConfig{
		NodeURLs:        []string{srvA.URL, srvB.URL},
		SubmitURL:       srvA.URL,
		LogPath:         logPath,
		DivergenceFile:  divPath,
		MaxStep:         1, // submit step 0 and step 1 — covers submit #2 (the diverging one)
		CorpusDir:       dir,
		SkipFund:        true,
		SkipSetup:       true,
		ValidateTimeout: 1 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := Shrink(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Matched {
		t.Fatalf("expected matched=true at MaxStep=1, got %+v", res)
	}
	if res.MatchedAt != 1 {
		t.Fatalf("expected MatchedAt=1, got %d", res.MatchedAt)
	}
	if res.TxsSubmitted != 2 {
		t.Fatalf("expected 2 submits, got %d", res.TxsSubmitted)
	}

	// Result file should be written under <CorpusDir>/shrinks/.
	files, _ := os.ReadDir(filepath.Join(dir, "shrinks"))
	if len(files) != 1 {
		t.Fatalf("expected 1 shrink result file, got %d", len(files))
	}
}

func TestShrink_PrefixDoesNotMatch(t *testing.T) {
	var submitsA, submitsB atomic.Int64
	srvA := httptest.NewServer(stubShrinkHandler("A", 0, &submitsA))
	defer srvA.Close()
	srvB := httptest.NewServer(stubShrinkHandler("B", 2, &submitsB))
	defer srvB.Close()

	dir := t.TempDir()
	logPath, divPath := writeShrinkInputs(t, dir,
		[]string{
			`{"step":0,"tx_type":"Payment","fields":{"TransactionType":"Payment","Account":"rA","Destination":"rB","Amount":"1"},"secret":"s"}`,
			`{"step":1,"tx_type":"Payment","fields":{"TransactionType":"Payment","Account":"rC","Destination":"rA","Amount":"2"},"secret":"s"}`,
		},
		corpus.Divergence{Kind: "tx_result", Details: map[string]any{"tx_type": "Payment"}},
	)

	cfg := ShrinkConfig{
		NodeURLs:        []string{srvA.URL, srvB.URL},
		SubmitURL:       srvA.URL,
		LogPath:         logPath,
		DivergenceFile:  divPath,
		MaxStep:         0, // only step 0 → submit #1 → no divergence
		CorpusDir:       dir,
		SkipFund:        true,
		SkipSetup:       true,
		ValidateTimeout: 1 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := Shrink(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if res.Matched {
		t.Fatalf("expected matched=false at MaxStep=0, got %+v", res)
	}
	if res.TxsSubmitted != 1 {
		t.Fatalf("expected 1 submit, got %d", res.TxsSubmitted)
	}
}

func TestShrink_RejectsTooFewNodes(t *testing.T) {
	dir := t.TempDir()
	logPath, divPath := writeShrinkInputs(t, dir,
		[]string{`{"step":0,"tx_type":"Payment","fields":{},"secret":"s"}`},
		corpus.Divergence{Kind: "tx_result", Details: map[string]any{"tx_type": "Payment"}},
	)
	cfg := ShrinkConfig{
		NodeURLs:       []string{"http://x"},
		SubmitURL:      "http://x",
		LogPath:        logPath,
		DivergenceFile: divPath,
		MaxStep:        0,
		CorpusDir:      dir,
		SkipFund:       true,
		SkipSetup:      true,
	}
	if _, err := Shrink(context.Background(), cfg); err == nil {
		t.Fatal("expected error for <2 nodes")
	}
}
