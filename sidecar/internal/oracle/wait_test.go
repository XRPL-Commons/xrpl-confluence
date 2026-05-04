package oracle

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

func TestWaitTxValidated_AllNodesValidated(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct{ Method string }
		_ = json.Unmarshal(body, &req)
		if req.Method == "tx" {
			n := hits.Add(1)
			validated := n >= 2 // first call: not yet; subsequent: yes
			resp := map[string]any{
				"result": map[string]any{
					"meta":      map[string]any{"TransactionResult": "tesSUCCESS", "AffectedNodes": []any{}},
					"validated": validated,
				},
			}
			b, _ := json.Marshal(resp)
			_, _ = w.Write(b)
			return
		}
		_, _ = w.Write([]byte(`{"result":{"status":"success"}}`))
	}))
	defer srv.Close()

	o := New([]Node{
		{Name: "a", Client: rpcclient.New(srv.URL)},
		{Name: "b", Client: rpcclient.New(srv.URL)},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := o.WaitTxValidated(ctx, "HASH", 3*time.Second, 50*time.Millisecond); err != nil {
		t.Fatalf("WaitTxValidated: %v", err)
	}
}

func TestWaitTxValidated_TimesOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":{"meta":{"TransactionResult":"tesSUCCESS","AffectedNodes":[]},"validated":false}}`))
	}))
	defer srv.Close()

	o := New([]Node{{Name: "a", Client: rpcclient.New(srv.URL)}})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := o.WaitTxValidated(ctx, "HASH", 200*time.Millisecond, 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestWaitTxValidated_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":{"meta":{"TransactionResult":"tesSUCCESS","AffectedNodes":[]},"validated":false}}`))
	}))
	defer srv.Close()

	o := New([]Node{{Name: "a", Client: rpcclient.New(srv.URL)}})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	err := o.WaitTxValidated(ctx, "HASH", 5*time.Second, 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
}
