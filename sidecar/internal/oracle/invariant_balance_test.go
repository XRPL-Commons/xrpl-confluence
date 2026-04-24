package oracle

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

func accountInfoServer(balances map[string]string) *httptest.Server {
	var mu sync.Mutex
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		var req struct {
			Method string           `json:"method"`
			Params []map[string]any `json:"params"`
		}
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		_ = json.Unmarshal(body, &req)
		if req.Method != "account_info" {
			_, _ = w.Write([]byte(`{"result":{"status":"error"}}`))
			return
		}
		var acct string
		if len(req.Params) > 0 {
			acct, _ = req.Params[0]["account"].(string)
		}
		bal, ok := balances[acct]
		if !ok {
			bal = "0"
		}
		_, _ = w.Write([]byte(`{"result":{"account_data":{"Account":"` + acct + `","Balance":"` + bal + `","Sequence":1},"status":"success"}}`))
	}))
}

func TestInvariantPoolBalance_AcceptsDecrease(t *testing.T) {
	srv := accountInfoServer(map[string]string{"rA": "1000", "rB": "500"})
	defer srv.Close()
	inv := NewInvariantPoolBalance([]string{"rA", "rB"})
	if err := inv.CheckLedger(rpcclient.New(srv.URL)); err != nil {
		t.Fatalf("first check: %v", err)
	}
	srv.Close()
	srv2 := accountInfoServer(map[string]string{"rA": "900", "rB": "500"})
	defer srv2.Close()
	if err := inv.CheckLedger(rpcclient.New(srv2.URL)); err != nil {
		t.Fatalf("decrease rejected: %v", err)
	}
}

func TestInvariantPoolBalance_RejectsIncrease(t *testing.T) {
	srv := accountInfoServer(map[string]string{"rA": "1000"})
	defer srv.Close()
	inv := NewInvariantPoolBalance([]string{"rA"})
	if err := inv.CheckLedger(rpcclient.New(srv.URL)); err != nil {
		t.Fatalf("first: %v", err)
	}
	srv.Close()
	srv2 := accountInfoServer(map[string]string{"rA": "1001"})
	defer srv2.Close()
	if err := inv.CheckLedger(rpcclient.New(srv2.URL)); err == nil {
		t.Fatal("expected error for balance increase")
	}
}

func TestInvariantPoolBalance_FirstCheckAlwaysOK(t *testing.T) {
	srv := accountInfoServer(map[string]string{"rA": "42"})
	defer srv.Close()
	inv := NewInvariantPoolBalance([]string{"rA"})
	if err := inv.CheckLedger(rpcclient.New(srv.URL)); err != nil {
		t.Fatal(err)
	}
}
