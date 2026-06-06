package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// setNodes directly injects node state into the poller, bypassing polling.
func setNodes(p *NodePoller, nodes map[string]Node) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.nodes = nodes
}

func makePoller(nodes map[string]Node) *NodePoller {
	cfgs := make([]NodeConfig, 0, len(nodes))
	for name, n := range nodes {
		cfgs = append(cfgs, NodeConfig{Name: name, Type: n.Type})
	}
	p := NewNodePoller(cfgs, time.Hour)
	setNodes(p, nodes)
	return p
}

// setHistory injects validated (seq -> hash) observations into the poller's
// history window, simulating what poll() accumulates over time.
func setHistory(p *NodePoller, node string, seqHashes map[int]string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for seq, hash := range seqHashes {
		p.recordValidatedLocked(node, seq, hash)
	}
}

func doStateDiff(t *testing.T, srv *Server, query string) (int, StateDiffResponse) {
	t.Helper()
	url := "/v1/state/diff"
	if query != "" {
		url += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	var resp StateDiffResponse
	if rec.Code == http.StatusOK {
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return rec.Code, resp
}

func TestStateDiff_NoPoller(t *testing.T) {
	srv := New()
	code, resp := doStateDiff(t, srv, "")
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if resp.Ledger != 0 {
		t.Errorf("ledger: got %d want 0", resp.Ledger)
	}
	if len(resp.HashByNode) != 0 {
		t.Errorf("hash_by_node: want empty, got %v", resp.HashByNode)
	}
	if resp.Diverged {
		t.Error("diverged: want false")
	}
	if resp.AsOf == 0 {
		t.Error("as_of must be non-zero")
	}
}

func TestStateDiff_ThreeAgreeingNodes(t *testing.T) {
	nodes := map[string]Node{
		"a": {Name: "a", Type: "rippled", ValidatedLedger: &LedgerRef{Seq: 100, Hash: "HASH100"}},
		"b": {Name: "b", Type: "rippled", ValidatedLedger: &LedgerRef{Seq: 100, Hash: "HASH100"}},
		"c": {Name: "c", Type: "rippled", ValidatedLedger: &LedgerRef{Seq: 100, Hash: "HASH100"}},
	}
	srv := New(WithNodePoller(makePoller(nodes)))

	code, resp := doStateDiff(t, srv, "")
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if resp.Ledger != 100 {
		t.Errorf("ledger: got %d want 100", resp.Ledger)
	}
	if len(resp.HashByNode) != 3 {
		t.Errorf("hash_by_node: want 3 entries, got %d", len(resp.HashByNode))
	}
	if resp.Diverged {
		t.Error("diverged: want false for 3 agreeing nodes")
	}
}

func TestStateDiff_DivergenceAndAtParam(t *testing.T) {
	nodes := map[string]Node{
		"a": {Name: "a", Type: "rippled", ValidatedLedger: &LedgerRef{Seq: 100, Hash: "HASH_A"}},
		"b": {Name: "b", Type: "rippled", ValidatedLedger: &LedgerRef{Seq: 100, Hash: "HASH_B"}},
		"c": {Name: "c", Type: "rippled", ValidatedLedger: &LedgerRef{Seq: 101, Hash: "HASH_C"}},
	}
	srv := New(WithNodePoller(makePoller(nodes)))

	// No at param: most common seq is 100 (count=2 vs 1), diverged=true.
	code, resp := doStateDiff(t, srv, "")
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if resp.Ledger != 100 {
		t.Errorf("ledger: got %d want 100 (most common)", resp.Ledger)
	}
	if len(resp.HashByNode) != 2 {
		t.Errorf("hash_by_node: want 2 entries at seq 100, got %d", len(resp.HashByNode))
	}
	if !resp.Diverged {
		t.Error("diverged: want true for 2 different hashes at seq 100")
	}

	// at=101: only node c.
	code, resp = doStateDiff(t, srv, "at=101")
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if resp.Ledger != 101 {
		t.Errorf("ledger: got %d want 101", resp.Ledger)
	}
	if len(resp.HashByNode) != 1 {
		t.Errorf("hash_by_node: want 1 entry at seq 101, got %d", len(resp.HashByNode))
	}
	if resp.Diverged {
		t.Error("diverged: want false for single node")
	}
}

func TestStateDiff_BadAtParam(t *testing.T) {
	srv := New()
	req := httptest.NewRequest(http.MethodGet, "/v1/state/diff?at=abc", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	var errResp map[string]map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	e := errResp["error"]
	if e["code"] != "bad_request" {
		t.Errorf("error code: got %q want bad_request", e["code"])
	}
}

// TestStateDiff_WedgeForkAtCommonSeq is the regression guard for the
// "front-end says nothing when there IS a desync" half of the bug: a node
// forks and wedges at seq 100 while the rest of the fleet advances to 105.
// The modal tip (105) is clean, so the old tip-only comparison reported
// Diverged=false. With history, the conflict at the common seq 100 surfaces.
func TestStateDiff_WedgeForkAtCommonSeq(t *testing.T) {
	nodes := map[string]Node{
		"a": {Name: "a", Type: "goxrpl", ValidatedLedger: &LedgerRef{Seq: 100, Hash: "FORK_A"}},
		"b": {Name: "b", Type: "rippled", ValidatedLedger: &LedgerRef{Seq: 105, Hash: "GOOD_105"}},
		"c": {Name: "c", Type: "rippled", ValidatedLedger: &LedgerRef{Seq: 105, Hash: "GOOD_105"}},
	}
	p := makePoller(nodes)
	// a forked at 100; b and c validated the canonical 100 before moving on.
	setHistory(p, "a", map[int]string{100: "FORK_A"})
	setHistory(p, "b", map[int]string{100: "GOOD_100", 105: "GOOD_105"})
	setHistory(p, "c", map[int]string{100: "GOOD_100", 105: "GOOD_105"})
	srv := New(WithNodePoller(p))

	code, resp := doStateDiff(t, srv, "")
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if !resp.Diverged {
		t.Fatal("diverged: want true — node a forked at seq 100")
	}
	if resp.Ledger != 100 {
		t.Errorf("ledger: got %d want 100 (the fork point)", resp.Ledger)
	}
	if resp.HashByNode["a"] != "FORK_A" || resp.HashByNode["b"] != "GOOD_100" {
		t.Errorf("hash_by_node: want a=FORK_A b=GOOD_100, got %v", resp.HashByNode)
	}
}

// TestStateDiff_AtParamIgnoresHistoryScan confirms an explicit ?at= answers
// strictly about that seq and does not get overridden by a fork elsewhere.
func TestStateDiff_AtParamIgnoresHistoryScan(t *testing.T) {
	nodes := map[string]Node{
		"a": {Name: "a", Type: "goxrpl", ValidatedLedger: &LedgerRef{Seq: 100, Hash: "FORK_A"}},
		"b": {Name: "b", Type: "rippled", ValidatedLedger: &LedgerRef{Seq: 105, Hash: "GOOD_105"}},
	}
	p := makePoller(nodes)
	setHistory(p, "a", map[int]string{100: "FORK_A"})
	setHistory(p, "b", map[int]string{100: "GOOD_100", 105: "GOOD_105"})
	srv := New(WithNodePoller(p))

	// Ask specifically about seq 105 — only b is there, so it must be clean.
	code, resp := doStateDiff(t, srv, "at=105")
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if resp.Diverged {
		t.Error("diverged: want false at seq 105 despite the fork at 100")
	}
	if resp.Ledger != 105 {
		t.Errorf("ledger: got %d want 105", resp.Ledger)
	}
}

func TestStateDiff_AtParamNoMatch(t *testing.T) {
	nodes := map[string]Node{
		"a": {Name: "a", Type: "rippled", ValidatedLedger: &LedgerRef{Seq: 100, Hash: "HASH100"}},
	}
	srv := New(WithNodePoller(makePoller(nodes)))

	code, resp := doStateDiff(t, srv, "at=999")
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if resp.Ledger != 999 {
		t.Errorf("ledger: got %d want 999", resp.Ledger)
	}
	if len(resp.HashByNode) != 0 {
		t.Errorf("hash_by_node: want empty, got %v", resp.HashByNode)
	}
	if resp.Diverged {
		t.Error("diverged: want false for no nodes")
	}
}
