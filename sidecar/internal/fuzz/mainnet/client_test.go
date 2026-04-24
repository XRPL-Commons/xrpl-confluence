package mainnet

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_LedgerFetchesTxList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		if req["method"] != "ledger" {
			t.Fatalf("method = %v", req["method"])
		}
		_, _ = w.Write([]byte(`{"result":{"ledger":{"ledger_index":"80000000","transactions":[
			{"TransactionType":"Payment","Account":"rA","Destination":"rB","Amount":"1000"},
			{"TransactionType":"TrustSet","Account":"rC","LimitAmount":{"currency":"USD","issuer":"rA","value":"100"}}
		]},"status":"success"}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	txs, err := c.LedgerTransactions(80_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if len(txs) != 2 {
		t.Fatalf("want 2 txs, got %d", len(txs))
	}
	if txs[0]["TransactionType"] != "Payment" {
		t.Fatalf("first tx type = %v", txs[0]["TransactionType"])
	}
}

func TestClient_CurrentValidatedSeq(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":{"info":{"validated_ledger":{"seq":80123456}},"status":"success"}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	seq, err := c.CurrentValidatedSeq()
	if err != nil {
		t.Fatal(err)
	}
	if seq != 80123456 {
		t.Fatalf("seq = %d", seq)
	}
}
