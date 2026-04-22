package accounts

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

func TestFundFromGenesis_SubmitsOnePaymentPerAccount(t *testing.T) {
	var mu sync.Mutex
	var calls []map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		mu.Lock()
		calls = append(calls, req)
		mu.Unlock()

		_, _ = w.Write([]byte(`{"result":{"engine_result":"tesSUCCESS","engine_result_code":0,"engine_result_message":"","tx_json":{"hash":"ABCD"},"status":"success"}}`))
	}))
	defer srv.Close()

	client := rpcclient.New(srv.URL)
	pool, err := NewPool(0x1111, 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := FundFromGenesis(client, pool, 10_000_000_000); err != nil {
		t.Fatalf("FundFromGenesis: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 3 {
		t.Fatalf("got %d submit calls, want 3", len(calls))
	}
	for i, c := range calls {
		if c["method"] != "submit" {
			t.Fatalf("call %d: method = %v, want submit", i, c["method"])
		}
		params := c["params"].([]any)[0].(map[string]any)
		if params["secret"] != "snoPBrXtMeMyMHUVTgbuqAfg1SUTb" {
			t.Fatalf("call %d: secret = %v, want genesis", i, params["secret"])
		}
		txj := params["tx_json"].(map[string]any)
		if txj["TransactionType"] != "Payment" {
			t.Fatalf("call %d: tx type = %v, want Payment", i, txj["TransactionType"])
		}
		if dst, ok := txj["Destination"].(string); !ok || !strings.HasPrefix(dst, "r") {
			t.Fatalf("call %d: bad destination %v", i, txj["Destination"])
		}
	}
}

func TestFundFromGenesis_PropagatesNonSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":{"engine_result":"tecUNFUNDED_PAYMENT","engine_result_code":104,"engine_result_message":"unfunded","tx_json":{"hash":"X"},"status":"success"}}`))
	}))
	defer srv.Close()

	client := rpcclient.New(srv.URL)
	pool, _ := NewPool(0x2222, 2)
	err := FundFromGenesis(client, pool, 1)
	if err == nil {
		t.Fatal("expected error on tecUNFUNDED_PAYMENT, got nil")
	}
}
