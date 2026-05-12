package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNodesHandler(t *testing.T) {
	const cannedResponse = `{
		"result": {
			"info": {
				"server_state": "proposing",
				"build_version": "1.12.0",
				"uptime": 500,
				"peers": 3,
				"complete_ledgers": "1-500",
				"network_id": 2,
				"pubkey_node": "nPubKey123",
				"ledger_current_index": 501,
				"validated_ledger": {"seq": 500, "hash": "HASH500"},
				"closed_ledger": {"seq": 499, "hash": "HASH499"},
				"last_close": {"proposers": 4, "converge_time_s": 2.5}
			}
		}
	}`

	fakeNode := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(cannedResponse))
	}))
	defer fakeNode.Close()

	fakeDown := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer fakeDown.Close()

	cfg := []NodeConfig{
		{Name: "alpha", Type: "rippled", RPC: fakeNode.URL},
		{Name: "beta", Type: "goxrpl", RPC: fakeDown.URL},
	}
	poller := NewNodePoller(cfg, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	poller.Start(ctx)
	time.Sleep(150 * time.Millisecond)

	srv := New(WithNodePoller(poller))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/nodes", nil)
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: got %q want application/json", ct)
	}

	var resp NodesResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Timestamp == 0 {
		t.Error("timestamp must be non-zero")
	}
	if time.Since(time.UnixMilli(resp.Timestamp)) > 5*time.Second {
		t.Error("timestamp is stale")
	}
	if len(resp.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(resp.Nodes))
	}

	byName := make(map[string]Node, 2)
	for _, n := range resp.Nodes {
		byName[n.Name] = n
	}

	alpha := byName["alpha"]
	if alpha.Status != "ok" {
		t.Errorf("alpha status: %q", alpha.Status)
	}
	if alpha.ServerState != "proposing" {
		t.Errorf("alpha server_state: %q", alpha.ServerState)
	}
	if alpha.ValidatedLedger == nil || alpha.ValidatedLedger.Seq != 500 {
		t.Errorf("alpha validated_ledger: %+v", alpha.ValidatedLedger)
	}
	if alpha.LastClose == nil || alpha.LastClose.ConvergeTimeS != 2.5 {
		t.Errorf("alpha last_close: %+v", alpha.LastClose)
	}

	beta := byName["beta"]
	if beta.Status != "unreachable" {
		t.Errorf("beta status: %q", beta.Status)
	}
	if beta.Error == "" {
		t.Error("beta must have error set")
	}
}
