package oracle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// txFakeServer returns a fixed tx_result for a given txHash → lookup map.
func txFakeServer(results map[string]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string           `json:"method"`
			Params []map[string]any `json:"params"`
		}
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		_ = json.Unmarshal(body, &req)
		hash := ""
		if len(req.Params) > 0 {
			if h, ok := req.Params[0]["transaction"].(string); ok {
				hash = h
			}
		}
		res := results[hash]
		if res == "" {
			res = "tesSUCCESS"
		}
		_, _ = w.Write([]byte(`{"result":{"meta":{"TransactionResult":"` + res + `"},"validated":true}}`))
	}))
}

func TestCompareTxResults_AgreementAcrossNodes(t *testing.T) {
	hash := "HASHABC"
	srvA := txFakeServer(map[string]string{hash: "tesSUCCESS"})
	defer srvA.Close()
	srvB := txFakeServer(map[string]string{hash: "tesSUCCESS"})
	defer srvB.Close()

	nodes := []Node{
		{Name: "A", Client: rpcclient.New(srvA.URL)},
		{Name: "B", Client: rpcclient.New(srvB.URL)},
	}
	o := New(nodes)

	cmp := o.CompareTxResult(context.Background(), hash)
	if !cmp.Agreed {
		t.Fatalf("expected agreement, got %+v", cmp)
	}
}

func TestCompareTxResults_DivergenceDetected(t *testing.T) {
	hash := "HASHXYZ"
	srvA := txFakeServer(map[string]string{hash: "tesSUCCESS"})
	defer srvA.Close()
	srvB := txFakeServer(map[string]string{hash: "tecUNFUNDED_PAYMENT"})
	defer srvB.Close()

	nodes := []Node{
		{Name: "A", Client: rpcclient.New(srvA.URL)},
		{Name: "B", Client: rpcclient.New(srvB.URL)},
	}
	o := New(nodes)

	cmp := o.CompareTxResult(context.Background(), hash)
	if cmp.Agreed {
		t.Fatalf("expected divergence, got %+v", cmp)
	}
	if len(cmp.NodeResults) != 2 {
		t.Fatalf("want 2 node results, got %d", len(cmp.NodeResults))
	}
}
