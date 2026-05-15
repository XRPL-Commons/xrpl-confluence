package oracle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// newFakeRippledServer returns an httptest.Server that responds to
// server_info JSON-RPC calls. The seq is mutable via the returned setter so
// tests can simulate ledger progression / freeze.
func newFakeRippledServer(t *testing.T, initialSeq, peers int) (*httptest.Server, func(int)) {
	t.Helper()
	var seq int64 = int64(initialSeq)
	var peerCount int64 = int64(peers)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Method string `json:"method"`
		}
		_ = json.Unmarshal(body, &req)
		if req.Method != "server_info" {
			http.Error(w, "unexpected method "+req.Method, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w,
			`{"result":{"info":{"server_state":"proposing","build_version":"test","uptime":1,"peers":%d,"complete_ledgers":"1-%d","network_id":10000,"pubkey_node":"nx","ledger_current_index":%d,"validated_ledger":{"seq":%d,"hash":"abc"},"closed_ledger":{"seq":%d,"hash":"def"},"last_close":{"proposers":2,"converge_time_s":0.5}},"status":"success"}}`,
			atomic.LoadInt64(&peerCount),
			atomic.LoadInt64(&seq),
			atomic.LoadInt64(&seq)+1,
			atomic.LoadInt64(&seq),
			atomic.LoadInt64(&seq)+1,
		)
	}))
	setSeq := func(v int) { atomic.StoreInt64(&seq, int64(v)) }
	return srv, setSeq
}

func TestMonitor_FiresConsensusStallAfterThreshold(t *testing.T) {
	srv, releaseSeq := newFakeRippledServer(t, 7, 5)
	defer srv.Close()

	o := New([]Node{
		{Name: "node-0", Client: rpcclient.New(srv.URL)},
		{Name: "node-1", Client: rpcclient.New(srv.URL)},
	})

	var (
		mu     sync.Mutex
		events []*LivenessEvent
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go o.Monitor(ctx, LivenessConfig{
		SampleInterval:        20 * time.Millisecond,
		StallThreshold:        100 * time.Millisecond,
		CooldownBetweenEvents: 200 * time.Millisecond,
		OnEvent: func(e *LivenessEvent) {
			mu.Lock()
			events = append(events, e)
			mu.Unlock()
		},
	})

	time.Sleep(250 * time.Millisecond)
	mu.Lock()
	got := len(events)
	mu.Unlock()
	if got == 0 {
		t.Fatalf("expected at least one consensus_stall event, got 0")
	}
	if events[0].Kind != "consensus_stall" {
		t.Errorf("kind: got %q want consensus_stall", events[0].Kind)
	}
	if events[0].StallSeconds < 0.1 {
		t.Errorf("stall_seconds: got %v want >= 0.1", events[0].StallSeconds)
	}

	// Advance the seq continuously — no further stalls must fire while the
	// network is making progress. Driver pumps seq every 30ms.
	stopAdvance := make(chan struct{})
	go func() {
		s := 8
		tick := time.NewTicker(30 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stopAdvance:
				return
			case <-tick.C:
				s++
				releaseSeq(s)
			}
		}
	}()
	defer close(stopAdvance)

	// Give a tick for the monitor to pick up the first advance.
	time.Sleep(60 * time.Millisecond)
	mu.Lock()
	before := len(events)
	mu.Unlock()

	time.Sleep(400 * time.Millisecond) // > stallThreshold + cooldown
	mu.Lock()
	after := len(events)
	mu.Unlock()
	if after > before {
		t.Errorf("expected no more stall events while seq is advancing, got %d more (before=%d after=%d)", after-before, before, after)
	}
}

func TestMonitor_FiresPeerDropBelowMin(t *testing.T) {
	srv, _ := newFakeRippledServer(t, 100, 0)
	defer srv.Close()

	o := New([]Node{
		{Name: "node-0", Client: rpcclient.New(srv.URL)},
	})

	var fired int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go o.Monitor(ctx, LivenessConfig{
		SampleInterval:        20 * time.Millisecond,
		StallThreshold:        time.Hour,
		CooldownBetweenEvents: 1 * time.Second,
		MinExpectedPeers:      2,
		OnEvent: func(e *LivenessEvent) {
			if e.Kind == "peer_drop" {
				atomic.AddInt32(&fired, 1)
			}
		},
	})

	time.Sleep(150 * time.Millisecond)
	if atomic.LoadInt32(&fired) == 0 {
		t.Fatal("expected peer_drop event, got none")
	}
}
