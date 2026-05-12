package client_test

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/client"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/finding"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/server"
)

// cannedServerInfo returns a canned server_info JSON response.
func cannedServerInfo(hash string) string {
	return `{"result":{"info":{` +
		`"server_state":"proposing","build_version":"1.12.0","uptime":100,` +
		`"peers":3,"complete_ledgers":"1-100","network_id":1,` +
		`"pubkey_node":"nPub","ledger_current_index":101,` +
		`"validated_ledger":{"seq":100,"hash":"` + hash + `"},` +
		`"closed_ledger":{"seq":99,"hash":"CCCC"},` +
		`"last_close":{"proposers":4,"converge_time_s":2.0}` +
		`}}}`
}

// testFixture wires up a full fake server and returns a client pointed at it,
// plus a teardown func.
func testFixture(t *testing.T) (*client.Client, func()) {
	t.Helper()

	// Two fake rippled nodes.
	fakeA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(cannedServerInfo("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")))
	}))
	fakeB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(cannedServerInfo("BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB")))
	}))

	cfg := []server.NodeConfig{
		{Name: "rippled-0", Type: "rippled", RPC: fakeA.URL},
		{Name: "rippled-1", Type: "rippled", RPC: fakeB.URL},
	}
	poller := server.NewNodePoller(cfg, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	poller.Start(ctx)

	store := finding.NewStore()
	oracle := finding.NewDivergenceOracle(poller, store, 50*time.Millisecond)
	oracle.Start(ctx)

	bus := server.NewEventBus()
	poller.SetEventBus(bus)

	// Findings dir.
	findingsDir := t.TempDir()
	watcher := finding.NewDiskWatcher(findingsDir, store, 50*time.Millisecond)
	watcher.Start(ctx)

	// Scenarios dir with one valid scenario.
	scenariosDir := t.TempDir()
	scenarioYAML := `apiVersion: confluence/v1
kind: Scenario
metadata:
  name: client-test-scenario
  description: used by client tests
topology:
  rippled: {count: 1}
  goxrpl: {count: 1}
workload:
  kind: soak
budget:
  duration: 1m
`
	if err := os.WriteFile(filepath.Join(scenariosDir, "test.yaml"), []byte(scenarioYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	// Logs dir.
	logsDir := t.TempDir()
	logContent := "2026-05-12T14:00:00.000Z [info] node started\n2026-05-12T14:00:01.000Z [warn] peer connected\n"
	if err := os.WriteFile(filepath.Join(logsDir, "rippled-0.log"), []byte(logContent), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := server.New(
		server.WithNodePoller(poller),
		server.WithFindingStore(store),
		server.WithEventBus(bus),
		server.WithScenariosDir(scenariosDir),
		server.WithLogsDir(logsDir),
		server.WithScenario("client-test"),
	)
	ts := httptest.NewServer(srv.Handler())

	// Wait for the poller + oracle to produce divergence findings.
	time.Sleep(300 * time.Millisecond)

	c := client.New(ts.URL)

	teardown := func() {
		cancel()
		ts.Close()
		fakeA.Close()
		fakeB.Close()
	}
	return c, teardown
}

func TestClient_Healthz(t *testing.T) {
	c, teardown := testFixture(t)
	defer teardown()

	h, err := c.Healthz(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !h.OK {
		t.Error("expected ok=true")
	}
	if h.APIVersion != "confluence/v1" {
		t.Errorf("unexpected api_version: %q", h.APIVersion)
	}
	if h.Scenario != "client-test" {
		t.Errorf("unexpected scenario: %q", h.Scenario)
	}
}

func TestClient_Nodes(t *testing.T) {
	c, teardown := testFixture(t)
	defer teardown()

	nr, err := c.Nodes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(nr.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nr.Nodes))
	}
	for _, n := range nr.Nodes {
		if n.Status != "ok" {
			t.Errorf("node %q: status=%q, want ok", n.Name, n.Status)
		}
	}
}

func TestClient_Findings_All(t *testing.T) {
	c, teardown := testFixture(t)
	defer teardown()

	// Retry until the oracle has emitted at least one finding.
	var findings []api.Finding
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var err error
		findings, err = c.Findings(context.Background(), "", "", 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(findings) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(findings) == 0 {
		t.Fatal("expected at least one finding")
	}
}

func TestClient_Findings_FilterKind(t *testing.T) {
	c, teardown := testFixture(t)
	defer teardown()

	// Wait for divergence findings to appear.
	var all []api.Finding
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var err error
		all, err = c.Findings(context.Background(), "", "", 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(all) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(all) == 0 {
		t.Skip("no findings produced by oracle in time")
	}

	// Filter by state_divergence — should still find results.
	div, err := c.Findings(context.Background(), "", api.KindStateDivergence, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(div) == 0 {
		t.Fatal("expected state_divergence findings")
	}
	for _, f := range div {
		if f.Kind != api.KindStateDivergence {
			t.Errorf("unexpected kind %q", f.Kind)
		}
	}

	// Filter by a kind that doesn't exist — should return empty, not error.
	none, err := c.Findings(context.Background(), "", "node_crash", 0)
	if err != nil {
		t.Fatal(err)
	}
	_ = none // may or may not be empty; just checking no error
}

func TestClient_FindingByID(t *testing.T) {
	c, teardown := testFixture(t)
	defer teardown()

	// Wait for at least one finding.
	var all []api.Finding
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var err error
		all, err = c.Findings(context.Background(), "", "", 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(all) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(all) == 0 {
		t.Skip("no findings produced")
	}

	id := all[0].ID
	f, err := c.FindingByID(context.Background(), id)
	if err != nil {
		t.Fatalf("FindingByID(%q): %v", id, err)
	}
	if f.ID != id {
		t.Errorf("id mismatch: got %q, want %q", f.ID, id)
	}

	// Non-existent ID must return *ErrAPI{Status:404}.
	_, err = c.FindingByID(context.Background(), "fnd_no")
	if err == nil {
		t.Fatal("expected error for missing finding")
	}
	apiErr, ok := err.(*client.ErrAPI)
	if !ok {
		t.Fatalf("expected *client.ErrAPI, got %T: %v", err, err)
	}
	if apiErr.Status != http.StatusNotFound {
		t.Errorf("expected 404, got %d", apiErr.Status)
	}
	if apiErr.Err.Code != api.ErrCodeFindingNotFound {
		t.Errorf("expected code %q, got %q", api.ErrCodeFindingNotFound, apiErr.Err.Code)
	}
}

func TestClient_StateDiff_NoAt(t *testing.T) {
	c, teardown := testFixture(t)
	defer teardown()

	diff, err := c.StateDiff(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	// Two nodes with different hashes → diverged.
	if !diff.Diverged {
		t.Error("expected diverged=true")
	}
}

func TestClient_StateDiff_WithAt(t *testing.T) {
	c, teardown := testFixture(t)
	defer teardown()

	diff, err := c.StateDiff(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	// Both fake nodes report seq=100 → should appear in hash_by_node.
	if diff.Ledger != 100 {
		t.Errorf("expected ledger=100, got %d", diff.Ledger)
	}
}

func TestClient_Scenarios(t *testing.T) {
	c, teardown := testFixture(t)
	defer teardown()

	sr, err := c.Scenarios(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sr.Scenarios) == 0 {
		t.Fatal("expected at least one scenario")
	}
	found := false
	for _, s := range sr.Scenarios {
		if s.Name == "client-test-scenario" {
			found = true
		}
	}
	if !found {
		t.Error("client-test-scenario not in list")
	}
}

func TestClient_ValidateScenario_Valid(t *testing.T) {
	c, teardown := testFixture(t)
	defer teardown()

	sc := &api.Scenario{
		APIVersion: "confluence/v1",
		Kind:       "Scenario",
		Metadata:   api.ScenarioMetadata{Name: "my-test"},
		Topology:   api.Topology{Rippled: api.NodeGroup{Count: 1}},
		Workload:   api.Workload{Kind: "soak"},
		Budget:     api.Budget{Duration: "10m"},
	}

	vr, err := c.ValidateScenario(context.Background(), sc)
	if err != nil {
		t.Fatal(err)
	}
	if !vr.OK {
		t.Errorf("expected ok=true, errors: %v", vr.Errors)
	}
}

func TestClient_Logs(t *testing.T) {
	c, teardown := testFixture(t)
	defer teardown()

	body, err := c.Logs(context.Background(), "rippled-0", 0, "", false, 10)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()

	sc := bufio.NewScanner(body)
	var lines int
	for sc.Scan() {
		lines++
	}
	if lines == 0 {
		t.Fatal("expected at least one log line")
	}
}

func TestClient_Events(t *testing.T) {
	c, teardown := testFixture(t)
	defer teardown()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	body, err := c.Events(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()

	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 1<<16), 1<<20)
	var gotData bool
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "data:") {
			gotData = true
			break
		}
	}
	if !gotData {
		t.Fatal("no SSE data: line received")
	}
}

func TestClient_NetworkFailure(t *testing.T) {
	// Listen on a random port and immediately close it so no server answers.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	c := client.New("http://" + addr)
	_, err = c.Healthz(context.Background())
	if err == nil {
		t.Fatal("expected error for closed port")
	}
	if _, ok := err.(*client.ErrAPI); ok {
		t.Fatal("expected a network error, not *ErrAPI")
	}
}
