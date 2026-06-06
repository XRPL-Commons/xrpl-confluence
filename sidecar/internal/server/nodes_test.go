package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNodePoller_Success(t *testing.T) {
	const cannedResponse = `{
		"result": {
			"info": {
				"server_state": "proposing",
				"build_version": "1.12.0",
				"uptime": 3600,
				"peers": 7,
				"complete_ledgers": "1-1000",
				"network_id": 1,
				"pubkey_node": "n9KUjqxCr5FKThSNXdzb7oqN8rYwScB2dUnNqxQxbEA17JkaWy5x",
				"ledger_current_index": 1001,
				"validated_ledger": { "seq": 1000, "hash": "AABBCC" },
				"closed_ledger": { "seq": 999, "hash": "DDEEFF" },
				"last_close": { "proposers": 5, "converge_time_s": 3.1 }
			}
		}
	}`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(cannedResponse))
	}))
	defer ts.Close()

	cfg := []NodeConfig{{Name: "node-a", Type: "rippled", RPC: ts.URL}}
	p := NewNodePoller(cfg, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.Start(ctx)

	time.Sleep(150 * time.Millisecond)

	snap := p.Snapshot()
	if snap.Timestamp == 0 {
		t.Fatal("expected non-zero timestamp")
	}
	if time.Since(time.UnixMilli(snap.Timestamp)) > 5*time.Second {
		t.Fatal("timestamp is stale")
	}
	if len(snap.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(snap.Nodes))
	}
	n := snap.Nodes[0]
	if n.Name != "node-a" {
		t.Errorf("name: got %q want %q", n.Name, "node-a")
	}
	if n.Type != "rippled" {
		t.Errorf("type: got %q want %q", n.Type, "rippled")
	}
	if n.Status != "ok" {
		t.Errorf("status: got %q want %q", n.Status, "ok")
	}
	if n.ServerState != "proposing" {
		t.Errorf("server_state: got %q want %q", n.ServerState, "proposing")
	}
	if n.BuildVersion != "1.12.0" {
		t.Errorf("build_version: got %q want %q", n.BuildVersion, "1.12.0")
	}
	if n.Uptime != 3600 {
		t.Errorf("uptime: got %d want 3600", n.Uptime)
	}
	if n.Peers != 7 {
		t.Errorf("peers: got %d want 7", n.Peers)
	}
	if n.CompleteLedgers != "1-1000" {
		t.Errorf("complete_ledgers: got %q want %q", n.CompleteLedgers, "1-1000")
	}
	if n.ValidatedLedger == nil || n.ValidatedLedger.Seq != 1000 || n.ValidatedLedger.Hash != "AABBCC" {
		t.Errorf("validated_ledger: got %+v", n.ValidatedLedger)
	}
	if n.ClosedLedger == nil || n.ClosedLedger.Seq != 999 || n.ClosedLedger.Hash != "DDEEFF" {
		t.Errorf("closed_ledger: got %+v", n.ClosedLedger)
	}
	if n.LedgerCurrentIndex != 1001 {
		t.Errorf("ledger_current_index: got %d want 1001", n.LedgerCurrentIndex)
	}
	if n.NetworkID != 1 {
		t.Errorf("network_id: got %d want 1", n.NetworkID)
	}
	if n.PubkeyNode != "n9KUjqxCr5FKThSNXdzb7oqN8rYwScB2dUnNqxQxbEA17JkaWy5x" {
		t.Errorf("pubkey_node: got %q", n.PubkeyNode)
	}
	if n.LastClose == nil || n.LastClose.Proposers != 5 {
		t.Errorf("last_close.proposers: got %+v", n.LastClose)
	}
	if n.Error != "" {
		t.Errorf("expected no error, got %q", n.Error)
	}
}

func TestNodePoller_Unreachable(t *testing.T) {
	ts500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer ts500.Close()

	const cannedResponse = `{"result":{"info":{"server_state":"full","peers":1,"build_version":"1.0"}}}`
	tsOK := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(cannedResponse))
	}))
	defer tsOK.Close()

	cfg := []NodeConfig{
		{Name: "ok-node", Type: "rippled", RPC: tsOK.URL},
		{Name: "bad-node", Type: "goxrpl", RPC: ts500.URL},
	}
	p := NewNodePoller(cfg, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.Start(ctx)

	time.Sleep(150 * time.Millisecond)

	snap := p.Snapshot()
	if len(snap.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(snap.Nodes))
	}

	byName := make(map[string]Node, 2)
	for _, n := range snap.Nodes {
		byName[n.Name] = n
	}

	ok := byName["ok-node"]
	if ok.Status != "ok" {
		t.Errorf("ok-node status: got %q want ok", ok.Status)
	}
	if ok.ServerState != "full" {
		t.Errorf("ok-node server_state: got %q want full", ok.ServerState)
	}

	bad := byName["bad-node"]
	if bad.Status != "unreachable" {
		t.Errorf("bad-node status: got %q want unreachable", bad.Status)
	}
	if bad.Error == "" {
		t.Error("bad-node should have error set")
	}
}

func TestNodePoller_JSONShape(t *testing.T) {
	const cannedResponse = `{"result":{"info":{"server_state":"full","peers":2}}}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(cannedResponse))
	}))
	defer ts.Close()

	cfg := []NodeConfig{{Name: "n1", Type: "rippled", RPC: ts.URL}}
	p := NewNodePoller(cfg, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.Start(ctx)
	time.Sleep(150 * time.Millisecond)

	snap := p.Snapshot()
	b, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := m["timestamp"]; !ok {
		t.Error("missing timestamp in JSON")
	}
	if _, ok := m["nodes"]; !ok {
		t.Error("missing nodes in JSON")
	}
}

// TestNodePoller_DivergenceSnapshotSpansHistory verifies the snapshot fed to
// the divergence oracle covers the validated-history window, not just current
// tips — so a wedge fork (node frozen at the fork seq while the fleet advances)
// still produces two conflicting hashes at the common seq.
func TestNodePoller_DivergenceSnapshotSpansHistory(t *testing.T) {
	p := NewNodePoller([]NodeConfig{
		{Name: "a", Type: "goxrpl"},
		{Name: "b", Type: "rippled"},
	}, time.Hour)
	p.mu.Lock()
	p.recordValidatedLocked("a", 100, "FORK_A") // a forked and wedged at 100
	p.recordValidatedLocked("b", 100, "GOOD_100")
	p.recordValidatedLocked("b", 105, "GOOD_105") // b advanced past the fork
	p.mu.Unlock()

	got := p.DivergenceSnapshot()
	hashesAt100 := map[string]bool{}
	for _, in := range got {
		if in.Seq == 100 {
			hashesAt100[in.Hash] = true
		}
	}
	if !hashesAt100["FORK_A"] || !hashesAt100["GOOD_100"] {
		t.Fatalf("seq 100 must carry both forked hashes, got %v (full: %v)", hashesAt100, got)
	}
}

// TestNodePoller_HistoryPrunes keeps the window bounded so a long run cannot
// grow the per-node history without limit.
func TestNodePoller_HistoryPrunes(t *testing.T) {
	p := NewNodePoller([]NodeConfig{{Name: "a", Type: "rippled"}}, time.Hour)
	p.mu.Lock()
	for seq := 1; seq <= maxValidatedHistorySeqs+200; seq++ {
		p.recordValidatedLocked("a", seq, "h")
	}
	n := len(p.history["a"])
	p.mu.Unlock()
	if n > maxValidatedHistorySeqs {
		t.Fatalf("history not pruned: %d entries, want <= %d", n, maxValidatedHistorySeqs)
	}
}
