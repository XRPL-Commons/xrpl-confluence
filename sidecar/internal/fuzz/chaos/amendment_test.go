package chaos

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

func TestAmendmentFlipEvent_VotesYesThenNo(t *testing.T) {
	calls := []map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string           `json:"method"`
			Params []map[string]any `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if len(req.Params) > 0 {
			calls = append(calls, req.Params[0])
		}
		_, _ = w.Write([]byte(`{"result":{"status":"success"}}`))
	}))
	defer srv.Close()

	cl := rpcclient.New(srv.URL)
	e := NewAmendmentFlipEvent(cl, "FeatureFoo")

	if !strings.HasPrefix(e.Name(), "amendment:FeatureFoo") {
		t.Fatalf("name = %q", e.Name())
	}
	if err := e.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("got %d calls, want 2", len(calls))
	}
	if calls[0]["feature"] != "FeatureFoo" || calls[0]["vetoed"] != false {
		t.Errorf("apply call = %+v", calls[0])
	}
	if calls[1]["feature"] != "FeatureFoo" || calls[1]["vetoed"] != true {
		t.Errorf("recover call = %+v", calls[1])
	}
}
