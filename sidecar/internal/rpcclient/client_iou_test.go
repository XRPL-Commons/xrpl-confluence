package rpcclient

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSubmitTxJSON_SendsArbitraryTxJSON(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"result":{"engine_result":"tesSUCCESS","engine_result_code":0,"engine_result_message":"","tx_json":{"hash":"ABCD"},"status":"success"}}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	txj := map[string]any{
		"TransactionType": "AccountSet",
		"Account":         "rX",
		"SetFlag":         uint32(5),
	}
	res, err := c.SubmitTxJSON("secret", txj)
	if err != nil {
		t.Fatalf("SubmitTxJSON: %v", err)
	}
	if res.EngineResult != "tesSUCCESS" {
		t.Fatalf("EngineResult = %q", res.EngineResult)
	}

	var req map[string]any
	_ = json.Unmarshal(capturedBody, &req)
	params := req["params"].([]any)[0].(map[string]any)
	got := params["tx_json"].(map[string]any)
	if got["TransactionType"] != "AccountSet" || got["Account"] != "rX" {
		t.Fatalf("bad tx_json: %v", got)
	}
	if got["SetFlag"] == nil {
		t.Fatalf("SetFlag missing")
	}
}

func TestSubmitPaymentIOU_SendsIOUAmountStructure(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"result":{"engine_result":"tesSUCCESS","engine_result_code":0,"engine_result_message":"","tx_json":{"hash":"ABCD"},"status":"success"}}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	amt := map[string]any{
		"currency": "USD",
		"issuer":   "rIssuer",
		"value":    "100",
	}
	res, err := c.SubmitPaymentIOU("secret123", "rSender", "rReceiver", amt)
	if err != nil {
		t.Fatalf("SubmitPaymentIOU: %v", err)
	}
	if res.EngineResult != "tesSUCCESS" {
		t.Fatalf("EngineResult = %q, want tesSUCCESS", res.EngineResult)
	}

	var req map[string]any
	if err := json.Unmarshal(capturedBody, &req); err != nil {
		t.Fatal(err)
	}
	params := req["params"].([]any)[0].(map[string]any)
	if params["secret"] != "secret123" {
		t.Fatalf("secret = %v", params["secret"])
	}
	txj := params["tx_json"].(map[string]any)
	if txj["TransactionType"] != "Payment" {
		t.Fatalf("type = %v", txj["TransactionType"])
	}
	gotAmt, ok := txj["Amount"].(map[string]any)
	if !ok {
		t.Fatalf("Amount = %T, want map", txj["Amount"])
	}
	if gotAmt["currency"] != "USD" || gotAmt["issuer"] != "rIssuer" || gotAmt["value"] != "100" {
		t.Fatalf("Amount content = %v", gotAmt)
	}
}
