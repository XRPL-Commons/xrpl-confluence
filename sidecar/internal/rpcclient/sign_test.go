package rpcclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSubmitTxBlob_PostsBlobMethod(t *testing.T) {
	captured := map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string           `json:"method"`
			Params []map[string]any `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		captured["method"] = req.Method
		if len(req.Params) > 0 {
			for k, v := range req.Params[0] {
				captured[k] = v
			}
		}
		_, _ = w.Write([]byte(`{"result":{"engine_result":"tesSUCCESS","tx_json":{"hash":"ABC"}}}`))
	}))
	defer srv.Close()

	cl := New(srv.URL)
	res, err := cl.SubmitTxBlob("DEADBEEF")
	if err != nil {
		t.Fatal(err)
	}
	if captured["method"] != "submit" {
		t.Errorf("method = %v, want submit", captured["method"])
	}
	if captured["tx_blob"] != "DEADBEEF" {
		t.Errorf("tx_blob = %v, want DEADBEEF", captured["tx_blob"])
	}
	if res.EngineResult != "tesSUCCESS" {
		t.Errorf("engine = %v", res.EngineResult)
	}
}

func TestSignLocal_AutofillsSequenceAndFee(t *testing.T) {
	calls := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		calls = append(calls, req.Method)
		switch req.Method {
		case "account_info":
			_, _ = w.Write([]byte(`{"result":{"account_data":{"Account":"rAddr","Balance":"1000000000","Sequence":42}}}`))
		case "ledger_current":
			_, _ = w.Write([]byte(`{"result":{"ledger_current_index":100}}`))
		default:
			_, _ = w.Write([]byte(`{"result":{}}`))
		}
	}))
	defer srv.Close()

	cl := New(srv.URL)
	tx := map[string]any{
		"TransactionType": "AccountSet",
		"Account":         "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh",
	}
	blob, err := cl.SignLocal("snoPBrXtMeMyMHUVTgbuqAfg1SUTb", tx)
	if err != nil {
		t.Fatal(err)
	}
	if blob == "" {
		t.Fatal("blob is empty")
	}
	wantCalls := map[string]bool{"account_info": false, "ledger_current": false}
	for _, c := range calls {
		if _, ok := wantCalls[c]; ok {
			wantCalls[c] = true
		}
	}
	if !wantCalls["account_info"] || !wantCalls["ledger_current"] {
		t.Errorf("calls = %v, want both account_info and ledger_current", calls)
	}
}
