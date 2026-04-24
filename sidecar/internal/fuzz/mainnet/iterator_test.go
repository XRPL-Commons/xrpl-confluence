package mainnet

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIterator_WalksRange(t *testing.T) {
	seqCalls := []int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		params := req["params"].([]any)[0].(map[string]any)
		seq := int(params["ledger_index"].(float64))
		seqCalls = append(seqCalls, seq)
		_, _ = w.Write([]byte(`{"result":{"ledger":{"transactions":[
			{"TransactionType":"Payment","Account":"rX"}
		]},"status":"success"}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	it := NewIterator(c, 100, 102)

	total := 0
	for it.Next() {
		tx := it.Tx()
		if tx["TransactionType"] != "Payment" {
			t.Fatalf("bad tx: %v", tx)
		}
		total++
	}
	if it.Err() != nil {
		t.Fatal(it.Err())
	}
	if total != 3 {
		t.Fatalf("txs = %d, want 3 (one per ledger)", total)
	}
	if len(seqCalls) != 3 || seqCalls[0] != 100 || seqCalls[2] != 102 {
		t.Fatalf("seqCalls = %v", seqCalls)
	}
}
