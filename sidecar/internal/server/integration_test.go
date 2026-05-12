package server_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/finding"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/server"
)

// cannedServerInfo returns a canned server_info JSON response with the given validated_ledger hash.
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

func TestControlServerEndToEnd(t *testing.T) {
	// --- fake rippled nodes ---
	fakeA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(cannedServerInfo("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")))
	}))
	defer fakeA.Close()

	fakeB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(cannedServerInfo("BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB")))
	}))
	defer fakeB.Close()

	// --- node poller ---
	cfg := []server.NodeConfig{
		{Name: "rippled-0", Type: "rippled", RPC: fakeA.URL},
		{Name: "rippled-1", Type: "rippled", RPC: fakeB.URL},
	}
	poller := server.NewNodePoller(cfg, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	poller.Start(ctx)

	// --- finding store + divergence oracle ---
	store := finding.NewStore()
	oracle := finding.NewDivergenceOracle(poller, store, 50*time.Millisecond)
	oracle.Start(ctx)

	// --- event bus ---
	bus := server.NewEventBus()
	poller.SetEventBus(bus)

	// --- findings dir (disk watcher) ---
	findingsDir := t.TempDir()
	watcher := finding.NewDiskWatcher(findingsDir, store, 50*time.Millisecond)
	watcher.Start(ctx)

	// --- scenarios dir ---
	scenariosDir := t.TempDir()
	scenarioYAML := `apiVersion: confluence/v1
kind: Scenario
metadata:
  name: integration-test-scenario
topology:
  rippled: {count: 1}
  goxrpl: {count: 1}
workload:
  kind: soak
budget:
  duration: 1m
  stop_on: [first_divergence]
`
	if err := os.WriteFile(filepath.Join(scenariosDir, "test.yaml"), []byte(scenarioYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	// --- logs dir ---
	logsDir := t.TempDir()
	logContent := "2026-05-12T14:00:00.000Z [info] node started\n2026-05-12T14:00:01.000Z [warn] peer connected\n"
	if err := os.WriteFile(filepath.Join(logsDir, "rippled-0.log"), []byte(logContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// --- server ---
	srv := server.New(
		server.WithNodePoller(poller),
		server.WithFindingStore(store),
		server.WithEventBus(bus),
		server.WithScenariosDir(scenariosDir),
		server.WithLogsDir(logsDir),
		server.WithScenario("integration-test"),
	)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Let the poller + oracle do at least 2 ticks.
	time.Sleep(200 * time.Millisecond)

	// healthz
	t.Run("healthz", func(t *testing.T) {
		resp := getJSON(t, ts.URL+"/v1/healthz")
		if resp["ok"] != true {
			t.Fatalf("ok != true, got: %v", resp["ok"])
		}
		if resp["api_version"] != "confluence/v1" {
			t.Fatalf("api_version: %v", resp["api_version"])
		}
		if resp["scenario"] != "integration-test" {
			t.Fatalf("scenario: %v", resp["scenario"])
		}
	})

	// nodes
	t.Run("nodes", func(t *testing.T) {
		resp := getJSON(t, ts.URL+"/v1/nodes")
		nodes, ok := resp["nodes"].([]any)
		if !ok {
			t.Fatalf("nodes is not an array: %T", resp["nodes"])
		}
		if len(nodes) != 2 {
			t.Fatalf("expected 2 nodes, got %d", len(nodes))
		}
		for _, raw := range nodes {
			n := raw.(map[string]any)
			if n["status"] != "ok" {
				t.Errorf("node %v status=%v, want ok", n["name"], n["status"])
			}
			vl, ok := n["validated_ledger"].(map[string]any)
			if !ok {
				t.Errorf("node %v: validated_ledger missing", n["name"])
				continue
			}
			// JSON numbers decode as float64.
			if seq, ok := vl["seq"].(float64); !ok || int(seq) != 100 {
				t.Errorf("node %v: validated_ledger.seq=%v, want 100", n["name"], vl["seq"])
			}
		}
	})

	// findings from the divergence oracle
	t.Run("findings_oracle", func(t *testing.T) {
		var arr []map[string]any
		deadline := time.Now().Add(1 * time.Second)
		for time.Now().Before(deadline) {
			arr = getJSONArray(t, ts.URL+"/v1/findings")
			if len(arr) > 0 {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if len(arr) == 0 {
			t.Fatal("expected at least one divergence finding from the oracle")
		}
		if arr[0]["kind"] != "state_divergence" {
			t.Fatalf("kind: %v", arr[0]["kind"])
		}
	})

	// findings from disk watcher
	t.Run("findings_disk", func(t *testing.T) {
		div := map[string]any{
			"seed":        42,
			"kind":        "crash",
			"description": "test crash",
			"details":     map[string]any{"node": "x"},
			"recorded_at": time.Now().UTC().Format(time.RFC3339Nano),
		}
		b, err := json.Marshal(div)
		if err != nil {
			t.Fatal(err)
		}
		divDir := filepath.Join(findingsDir, "divergences")
		if err := os.MkdirAll(divDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(divDir, "test-crash.json"), b, 0o644); err != nil {
			t.Fatal(err)
		}

		deadline := time.Now().Add(2 * time.Second)
		var found bool
		for time.Now().Before(deadline) {
			arr := getJSONArray(t, ts.URL+"/v1/findings?kind=node_crash")
			if len(arr) > 0 {
				found = true
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if !found {
			t.Fatal("disk-sourced crash finding never appeared")
		}
	})

	// findings by ID
	t.Run("finding_by_id", func(t *testing.T) {
		arr := getJSONArray(t, ts.URL+"/v1/findings?limit=1")
		if len(arr) == 0 {
			t.Skip("no findings to test by-id")
		}
		id, ok := arr[0]["id"].(string)
		if !ok || id == "" {
			t.Fatalf("id not a string: %v", arr[0]["id"])
		}
		resp := getJSON(t, ts.URL+"/v1/findings/"+id)
		if resp["id"] != id {
			t.Fatalf("id mismatch: got %v, want %s", resp["id"], id)
		}

		resp404 := getJSONStatus(t, ts.URL+"/v1/findings/fnd_nonexistent", 404)
		errObj, ok := resp404["error"].(map[string]any)
		if !ok {
			t.Fatalf("error not an object: %v", resp404["error"])
		}
		if errObj["code"] != "finding_not_found" {
			t.Fatalf("code: %v", errObj["code"])
		}
	})

	// state diff
	t.Run("state_diff", func(t *testing.T) {
		resp := getJSON(t, ts.URL+"/v1/state/diff")
		if resp["diverged"] != true {
			t.Fatalf("expected diverged=true (two fakes return different hashes), got: %v", resp["diverged"])
		}
	})

	// scenarios list
	t.Run("scenarios_list", func(t *testing.T) {
		resp := getJSON(t, ts.URL+"/v1/scenarios")
		list, ok := resp["scenarios"].([]any)
		if !ok {
			t.Fatalf("scenarios is not an array: %T", resp["scenarios"])
		}
		if len(list) == 0 {
			t.Fatal("expected at least one scenario")
		}
	})

	// scenarios validate
	t.Run("scenarios_validate_ok", func(t *testing.T) {
		body := `{"api_version":"confluence/v1","kind":"Scenario","metadata":{"name":"x"},"topology":{"rippled":{"count":1},"goxrpl":{"count":1}},"workload":{"kind":"soak"},"budget":{"duration":"1m","stop_on":["first_divergence"]}}`
		resp := postJSON(t, ts.URL+"/v1/scenarios/validate", body)
		if resp["ok"] != true {
			t.Fatalf("validate response: %v", resp)
		}
	})

	// events SSE
	t.Run("events", func(t *testing.T) {
		req, err := http.NewRequest("GET", ts.URL+"/v1/events", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Accept", "text/event-stream")
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("events GET: %v", err)
		}
		defer resp.Body.Close()

		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 1<<16), 1<<20)
		var got int
		deadline := time.Now().Add(1500 * time.Millisecond)
		for sc.Scan() && time.Now().Before(deadline) {
			line := sc.Text()
			if strings.HasPrefix(line, "data:") {
				got++
				if got >= 1 {
					break
				}
			}
		}
		if got == 0 {
			// The poller publishes node events on each tick; at least one should
			// arrive within the 1.5s window. A keepalive comment (":ok") counts
			// as proof the stream is live — check for that too.
			t.Fatal("no SSE data events received")
		}
	})

	// logs
	t.Run("logs", func(t *testing.T) {
		body := getStream(t, ts.URL+"/v1/logs?node=rippled-0&limit=10")
		defer body.Close()
		sc := bufio.NewScanner(body)
		var lines int
		for sc.Scan() {
			lines++
		}
		if lines == 0 {
			t.Fatal("no log lines returned")
		}
	})
}

// --- helpers ---

func getJSON(t *testing.T, url string) map[string]any {
	t.Helper()
	return getJSONStatus(t, url, http.StatusOK)
}

func getJSONStatus(t *testing.T, url string, wantStatus int) map[string]any {
	t.Helper()
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		t.Fatalf("GET %s: status %d, want %d", url, resp.StatusCode, wantStatus)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode JSON from %s: %v", url, err)
	}
	return out
}

func getJSONArray(t *testing.T, url string) []map[string]any {
	t.Helper()
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", url, resp.StatusCode)
	}
	var raw []any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatalf("decode JSON array from %s: %v", url, err)
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("array element is not an object: %T", item)
		}
		out = append(out, m)
	}
	return out
}

func postJSON(t *testing.T, url, body string) map[string]any {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body)) //nolint:noctx
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode JSON from POST %s: %v", url, err)
	}
	return out
}

func getStream(t *testing.T, url string) io.ReadCloser {
	t.Helper()
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("GET %s: status %d", url, resp.StatusCode)
	}
	return resp.Body
}
