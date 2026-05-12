package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/finding"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/server"
)

// minimalScenarioYAML is a valid scenario with a very short budget.
const minimalScenarioYAML = `apiVersion: confluence/v1
kind: Scenario
metadata:
  name: run-test
topology:
  rippled: {count: 1}
  goxrpl: {count: 1}
workload:
  kind: soak
budget:
  duration: 100ms
`

// newRunServer starts an in-process control server and returns its URL + teardown.
func newRunServer(t *testing.T) (string, func()) {
	t.Helper()
	store := finding.NewStore()
	bus := server.NewEventBus()
	srv := server.New(
		server.WithFindingStore(store),
		server.WithEventBus(bus),
	)
	ts := httptest.NewServer(srv.Handler())
	return ts.URL, ts.Close
}

// newRunCmdDeps builds upDeps/downDeps whose kurtosis CLI is a no-op fake and
// whose httpClient is redirected to the given control server URL.
func newRunCmdDeps(t *testing.T, controlURL string) (*upDeps, *downDeps) {
	t.Helper()

	srv := &httptest.Server{URL: controlURL}
	_ = srv

	fakeCLI := &fakeCLI{
		next: func(args []string) (string, string, error) {
			if len(args) >= 2 && args[0] == "service" && args[1] == "inspect" {
				// Return the host portion of controlURL as the IP address.
				host := strings.TrimPrefix(controlURL, "http://")
				if idx := strings.LastIndex(host, ":"); idx >= 0 {
					host = host[:idx]
				}
				return "UUID: test-uuid\nIP Address: " + host + "\n", "", nil
			}
			return "", "", nil
		},
	}

	// Create a redirecting HTTP client so health checks reach the test server.
	httpCli := redirectClient(&httptest.Server{URL: controlURL})

	up := &upDeps{
		cli:        fakeCLI,
		httpClient: httpCli,
	}
	down := &downDeps{cli: fakeCLI}
	return up, down
}

func runRunCmd(t *testing.T, up *upDeps, down *downDeps, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	outBuf, errBuf := &bytes.Buffer{}, &bytes.Buffer{}
	root := newRootCmd()
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	replaceSubCmd(root, newRunCmdWith(up, down))
	root.SetArgs(args)
	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

func TestRun_WaitCompletedBudget_JSON(t *testing.T) {
	withDiscoveryDir(t)

	controlURL, teardown := newRunServer(t)
	defer teardown()

	// Write scenario file.
	dir := t.TempDir()
	scenarioPath := filepath.Join(dir, "scenario.yaml")
	if err := os.WriteFile(scenarioPath, []byte(minimalScenarioYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	up, down := newRunCmdDeps(t, controlURL)

	stdout, _, err := runRunCmd(t, up, down,
		"run", scenarioPath,
		"--json",
		"--wait=true",
		"--down=false",
		"--tear-down-first=false",
		"--wait-control=5s",
		"--timeout=10s",
	)

	// exitCodeError(0) means success, exitCodeError(3) is also a valid exit
	// from the runner's perspective (findings / stop_on). Accept both.
	if err != nil {
		if _, ok := err.(exitCodeError); !ok {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	var got map[string]any
	if jerr := json.Unmarshal([]byte(stdout), &got); jerr != nil {
		t.Fatalf("output is not JSON: %v (got %q)", jerr, stdout)
	}

	status, _ := got["status"].(string)
	if status != server.RunStatusCompletedBudget {
		t.Errorf("expected status %q, got %q", server.RunStatusCompletedBudget, status)
	}
	durationMS, _ := got["duration_ms"].(float64)
	if durationMS <= 0 {
		t.Errorf("expected duration_ms > 0, got %v", durationMS)
	}
	if got["run_id"] == "" || got["run_id"] == nil {
		t.Errorf("expected non-empty run_id")
	}
}

func TestRun_ValidationFail(t *testing.T) {
	withDiscoveryDir(t)

	dir := t.TempDir()
	p := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(p, []byte("apiVersion: confluence/v1\nkind: Scenario\nmetadata:\n  name: bad\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	up, down := newRunCmdDeps(t, "http://127.0.0.1:1")

	stdout, _, err := runRunCmd(t, up, down, "run", p, "--json", "--down=false", "--tear-down-first=false")
	if err == nil {
		t.Fatal("expected non-nil error for invalid scenario")
	}

	var got struct {
		OK     bool             `json:"ok"`
		Errors []map[string]any `json:"errors"`
	}
	if jerr := json.Unmarshal([]byte(stdout), &got); jerr != nil {
		t.Fatalf("not JSON: %v (out=%q)", jerr, stdout)
	}
	if got.OK || len(got.Errors) == 0 {
		t.Errorf("expected validation errors, got %+v", got)
	}
}

// --- replay tests ---

func runReplayCmd(t *testing.T, up *upDeps, down *downDeps, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	outBuf, errBuf := &bytes.Buffer{}, &bytes.Buffer{}
	root := newRootCmd()
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	replaceSubCmd(root, newReplayCmdWith(up, down))
	root.SetArgs(args)
	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

func TestReplay_NotFound_Errors(t *testing.T) {
	withDiscoveryDir(t)
	tmp := t.TempDir()
	up, down := newRunCmdDeps(t, "http://127.0.0.1:1")

	_, _, err := runReplayCmd(t, up, down,
		"replay", "rep_missing",
		"--dest", tmp,
		"--down=false", "--tear-down-first=false",
	)
	if err == nil {
		t.Fatal("expected error for missing reproducer")
	}
	if !strings.Contains(err.Error(), "reproducer not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestReplay_LoadsReproducerAndRuns(t *testing.T) {
	withDiscoveryDir(t)

	controlURL, teardown := newRunServer(t)
	defer teardown()

	tmp := t.TempDir()
	// Create reproducer YAML at expected path.
	reproducerID := "rep_test"
	reproDir := filepath.Join(tmp, "reproducers")
	if err := os.MkdirAll(reproDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reproDir, reproducerID+".yaml"), []byte(minimalScenarioYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	up, down := newRunCmdDeps(t, controlURL)

	stdout, _, err := runReplayCmd(t, up, down,
		"replay", reproducerID,
		"--dest", tmp,
		"--json",
		"--wait=true",
		"--down=false",
		"--tear-down-first=false",
		"--wait-control=5s",
		"--timeout=10s",
	)

	if err != nil {
		if _, ok := err.(exitCodeError); !ok {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	var got map[string]any
	if jerr := json.Unmarshal([]byte(stdout), &got); jerr != nil {
		t.Fatalf("output is not JSON: %v (got %q)", jerr, stdout)
	}
	status, _ := got["status"].(string)
	if status != server.RunStatusCompletedBudget {
		t.Errorf("expected status %q, got %q", server.RunStatusCompletedBudget, status)
	}
}

// --- pull reproducer tests ---

func TestPull_Findings_AlsoCopiesReproducers(t *testing.T) {
	tmp := t.TempDir()

	docker := &fakeDockerExec{
		containers: []string{"confluence-control--uuid-ctrl-1"},
		copyPayload: map[string]string{
			"findings":    "finding-001.json",
			"reproducers": "rep-001.yaml",
		},
	}
	cli := &fakePullCLI{
		uuids: map[string]string{"confluence-control": "uuid-ctrl-1"},
	}

	outBuf := &bytes.Buffer{}
	root := newRootCmd()
	root.SetOut(outBuf)
	root.SetErr(&bytes.Buffer{})
	_ = root.PersistentFlags().Set("enclave", "myenclave")

	cmd := newPullCmd()
	root.AddCommand(cmd)
	cmd.SetOut(outBuf)
	_ = cmd.Flags().Set("findings", "true")
	_ = cmd.Flags().Set("corpus", "false")
	_ = cmd.Flags().Set("dest", tmp)
	cmd.SetContext(context.Background())

	if err := runPullWith(cmd, cli, docker); err != nil {
		t.Fatalf("runPullWith: %v", err)
	}

	// Should have 2 copy calls: findings + reproducers.
	if len(docker.copyCalls) != 2 {
		t.Fatalf("expected 2 copy calls (findings + reproducers), got %d: %+v", len(docker.copyCalls), docker.copyCalls)
	}

	var foundReproducers bool
	for _, call := range docker.copyCalls {
		if call.Src == "/var/confluence/reproducers/." {
			foundReproducers = true
			if !strings.HasSuffix(call.Dest, "reproducers") {
				t.Errorf("reproducers dest doesn't end in 'reproducers': %q", call.Dest)
			}
		}
	}
	if !foundReproducers {
		t.Error("no reproducers copy call found")
	}

	// Human output must mention reproducers.
	if !strings.Contains(outBuf.String(), "Pulled reproducers") {
		t.Errorf("expected 'Pulled reproducers' in output, got: %q", outBuf.String())
	}
}

func TestPull_JSON_IncludesReproducers(t *testing.T) {
	tmp := t.TempDir()

	docker := &fakeDockerExec{
		containers:  []string{"confluence-control--uuid-ctrl-1"},
		copyPayload: map[string]string{"findings": "f1.json", "reproducers": "r1.yaml"},
	}
	cli := &fakePullCLI{
		uuids: map[string]string{"confluence-control": "uuid-ctrl-1"},
	}

	outBuf := &bytes.Buffer{}
	root := newRootCmd()
	root.SetOut(outBuf)
	root.SetErr(&bytes.Buffer{})
	_ = root.PersistentFlags().Set("enclave", "myenclave")
	_ = root.PersistentFlags().Set("json", "true")

	cmd := newPullCmd()
	root.AddCommand(cmd)
	cmd.SetOut(outBuf)
	_ = cmd.Flags().Set("findings", "true")
	_ = cmd.Flags().Set("corpus", "false")
	_ = cmd.Flags().Set("dest", tmp)
	cmd.SetContext(context.Background())

	if err := runPullWith(cmd, cli, docker); err != nil {
		t.Fatalf("runPullWith: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(outBuf.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v (got %q)", err, outBuf.String())
	}
	copied, ok := got["copied"].([]any)
	if !ok {
		t.Fatalf("copied is not an array: %T", got["copied"])
	}
	// Expect findings + reproducers entries.
	if len(copied) != 2 {
		t.Errorf("expected 2 copied entries, got %d", len(copied))
	}
	var kinds []string
	for _, item := range copied {
		if m, ok := item.(map[string]any); ok {
			if k, ok := m["kind"].(string); ok {
				kinds = append(kinds, k)
			}
		}
	}
	hasReproducers := false
	for _, k := range kinds {
		if k == "reproducers" {
			hasReproducers = true
		}
	}
	if !hasReproducers {
		t.Errorf("reproducers entry missing from copied: %v", kinds)
	}
}

// --- exit code tests ---

func TestExitCodeForRun(t *testing.T) {
	cases := []struct {
		run      server.Run
		wantCode int
	}{
		{server.Run{Status: server.RunStatusCompletedBudget, FindingIDs: []string{}}, 0},
		{server.Run{Status: server.RunStatusCompletedBudget, FindingIDs: []string{"f1"}}, 3},
		{server.Run{Status: server.RunStatusCompletedStopOn, FindingIDs: []string{}}, 3},
		{server.Run{Status: server.RunStatusFailed, FindingIDs: []string{}}, 1},
	}
	for _, tc := range cases {
		got := exitCodeForRun(tc.run)
		if got != tc.wantCode {
			t.Errorf("exitCodeForRun(%+v) = %d, want %d", tc.run, got, tc.wantCode)
		}
	}
}

// newRunServer needs server.EventBus accessible. Add a helper to build a
// test server with a live scenario for the httptest-based tests.
func init() {
	// Ensure api package is used (FindingIDs uses api.Finding type).
	_ = api.Finding{}
	_ = time.Now()
}
