package rpcclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTx_ReturnsRawAffectedNodes(t *testing.T) {
	body := `{"result":{
		"meta":{
			"TransactionResult":"tesSUCCESS",
			"AffectedNodes":[
				{"CreatedNode":{"LedgerEntryType":"Offer","LedgerIndex":"DEADBEEF"}}
			]
		},
		"validated":true
	}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := New(srv.URL)
	res, err := c.Tx("DEADBEEF")
	if err != nil {
		t.Fatal(err)
	}
	if res.TransactionResult != "tesSUCCESS" {
		t.Fatalf("TxResult = %q", res.TransactionResult)
	}
	if !res.Validated {
		t.Fatal("expected Validated=true")
	}
	if len(res.AffectedNodes) == 0 {
		t.Fatalf("AffectedNodes empty")
	}
	var nodes []map[string]any
	if err := json.Unmarshal(res.AffectedNodes, &nodes); err != nil {
		t.Fatalf("AffectedNodes not valid JSON: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("nodes = %d, want 1", len(nodes))
	}
}
