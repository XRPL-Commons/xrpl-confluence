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
