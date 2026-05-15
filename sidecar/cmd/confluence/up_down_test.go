package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/discovery"
	"github.com/spf13/cobra"
)

// fakeCLI records invocations and returns canned output.
type fakeCLI struct {
	runs [][]string
	next func(args []string) (stdout, stderr string, err error)
}

func (f *fakeCLI) Run(_ context.Context, args []string, _ io.Reader, stdout, stderr io.Writer) error {
	f.runs = append(f.runs, append([]string(nil), args...))
	if f.next == nil {
		return nil
	}
	out, errOut, err := f.next(args)
	_, _ = io.WriteString(stdout, out)
	_, _ = io.WriteString(stderr, errOut)
	return err
}

// absTestdata returns the absolute path to a file under
// ../../internal/scenario/testdata/ relative to the package source dir.
func absTestdata(t *testing.T, name string) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Join(cwd, "..", "..", "internal", "scenario", "testdata", name)
}

// withDiscoveryDir changes the working directory to a temp dir so discovery
// file ops don't pollute the real filesystem. It restores cwd on cleanup.
func withDiscoveryDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

// healthzServer returns a running httptest.Server that replies 200 to /v1/healthz.
func healthzServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// redirectClient returns an *http.Client whose transport rewrites every
// request's host to the test server's host:port. This lets waitForControl
// build a URL like http://127.0.0.1:8090 which transparently hits srv.
func redirectClient(srv *httptest.Server) *http.Client {
	srvAddr := strings.TrimPrefix(srv.URL, "http://")
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &redirectTransport{target: srvAddr},
	}
}

type redirectTransport struct{ target string }

func (r *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Host = r.target
	return http.DefaultTransport.RoundTrip(clone)
}

// fakeCLIForUp returns a *fakeCLI whose next func handles:
//   - "run …"                    → success
//   - "service inspect <enc> …"  → returns output with the test server's host
//
// The httptest server listens on a random port, not 8090. To make the URL
// produced by waitForControl match the httptest server we embed host:port from
// serverURL directly into the "IP Address" field and override port 8090 by
// having the test http.Client (srv.Client()) route all traffic to the server.
// Simpler approach: we store the full addr in IP Address and use port 8090 in
// the format string; the httptest client transparently rewrites the target.
//
// Actually the cleanest approach: put the httptest's host in the IP field, and
// have the httpClient be the test server's own client which is configured to
// always talk to the right port. The control URL will be http://<host>:8090 but
// the server client ignores the port and routes to the test server.
// Unfortunately that isn't how httptest.Server.Client() works — it only sets
// the TLS cert pool.
//
// Cleanest real approach: store the full addr (host:port) in IP Address and
// strip port from the format to get http://<host:port> — i.e. use fmt.Sprintf
// but build the URL differently in the test. Instead, simply override the
// controlURL derivation by injecting a custom host that includes the port.
// We do this by setting IP Address to "host:port" (with colon) and using port 0
// so the URL becomes "http://host:port:0" which is wrong.
//
// Pragmatic solution: set the IP Address to the full <host> and store the
// real port so the URL is correct; but we need to intercept port 8090. We use a
// custom http.Transport that redirects all requests to the test server.
func fakeCLIForUp(t *testing.T, enclave, serverURL string) *fakeCLI {
	t.Helper()
	hostPort := strings.TrimPrefix(serverURL, "http://")
	host := hostPort
	if idx := strings.LastIndex(hostPort, ":"); idx >= 0 {
		host = hostPort[:idx]
	}
	inspectOut := fmt.Sprintf("UUID: test-uuid\nIP Address: %s\n", host)
	return &fakeCLI{
		next: func(args []string) (string, string, error) {
			if len(args) >= 2 && args[0] == "service" && args[1] == "inspect" {
				return inspectOut, "", nil
			}
			return "", "", nil
		},
	}
}

// runUpCmd runs the "up" command with the given args through a root cmd that
// has the provided upDeps injected. Returns stdout, stderr, and the error.
func runUpCmd(t *testing.T, deps *upDeps, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	outBuf, errBuf := &bytes.Buffer{}, &bytes.Buffer{}
	root := newRootCmd()
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	// Replace the up sub-command with one using our injected deps.
	replaceSubCmd(root, newUpCmdWith(deps))
	root.SetArgs(args)
	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

// runDownCmd injects downDeps into the root and runs args.
func runDownCmd(t *testing.T, deps *downDeps, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	outBuf, errBuf := &bytes.Buffer{}, &bytes.Buffer{}
	root := newRootCmd()
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	replaceSubCmd(root, newDownCmdWith(deps))
	root.SetArgs(args)
	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

// replaceSubCmd removes the existing sub-command with the same Use prefix and
// adds the replacement. This lets us inject deps without reconstructing the
// entire root.
func replaceSubCmd(root *cobra.Command, replacement *cobra.Command) {
	use := strings.Fields(replacement.Use)[0]
	for _, c := range root.Commands() {
		if strings.Fields(c.Use)[0] == use {
			root.RemoveCommand(c)
			break
		}
	}
	root.AddCommand(replacement)
}

// ── up tests ──────────────────────────────────────────────────────────────────

func TestUpJSON_HappyPath(t *testing.T) {
	scenarioPath := absTestdata(t, "soak.yaml")
	withDiscoveryDir(t)
	srv := healthzServer(t)

	cli := fakeCLIForUp(t, "soak-mixed-3x2", srv.URL)
	deps := &upDeps{
		cli:        cli,
		httpClient: redirectClient(srv),
	}

	stdout, _, err := runUpCmd(t, deps, "up", "--json", "--scenario", scenarioPath, "--wait-control", "5s")
	if err != nil {
		t.Fatalf("up: %v", err)
	}

	var got struct {
		EnclaveID  string    `json:"enclave_id"`
		ControlURL string    `json:"control_url"`
		Scenario   string    `json:"scenario"`
		StartedAt  time.Time `json:"started_at"`
	}
	if jerr := json.Unmarshal([]byte(stdout), &got); jerr != nil {
		t.Fatalf("not JSON: %v (out=%q)", jerr, stdout)
	}
	if got.EnclaveID == "" {
		t.Error("enclave_id empty")
	}
	if got.ControlURL == "" {
		t.Error("control_url empty")
	}
	if got.Scenario != "soak-mixed-3x2" {
		t.Errorf("scenario: got %q want %q", got.Scenario, "soak-mixed-3x2")
	}
	if got.StartedAt.IsZero() {
		t.Error("started_at zero")
	}

	// Discovery file must exist.
	cur, readErr := discovery.Read()
	if readErr != nil {
		t.Fatalf("discovery.Read: %v", readErr)
	}
	if cur.EnclaveID != got.EnclaveID {
		t.Errorf("discovery enclave_id mismatch: %q vs %q", cur.EnclaveID, got.EnclaveID)
	}

	// CLI must have received a "run" call.
	var gotRun bool
	for _, run := range cli.runs {
		if len(run) > 0 && run[0] == "run" {
			gotRun = true
			break
		}
	}
	if !gotRun {
		t.Errorf("expected kurtosis run call; got: %v", cli.runs)
	}
}

func TestUpHuman_HappyPath(t *testing.T) {
	scenarioPath := absTestdata(t, "soak.yaml")
	withDiscoveryDir(t)
	srv := healthzServer(t)

	cli := fakeCLIForUp(t, "soak-mixed-3x2", srv.URL)
	deps := &upDeps{cli: cli, httpClient: redirectClient(srv)}

	stdout, _, err := runUpCmd(t, deps, "up", "--scenario", scenarioPath, "--wait-control", "5s")
	if err != nil {
		t.Fatalf("up: %v", err)
	}
	if !strings.Contains(stdout, "ready at") {
		t.Errorf("expected human ready message, got %q", stdout)
	}
}

func TestUp_ValidationFail(t *testing.T) {
	withDiscoveryDir(t)

	// Write a YAML that will fail validation (no topology).
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(p, []byte("apiVersion: confluence/v1\nkind: Scenario\nmetadata:\n  name: bad\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cli := &fakeCLI{}
	deps := &upDeps{cli: cli, httpClient: http.DefaultClient}

	stdout, _, err := runUpCmd(t, deps, "up", "--json", "--scenario", p, "--wait-control", "1s")
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
		t.Errorf("expected errors, got %+v", got)
	}

	// Kurtosis must NOT have been called.
	for _, run := range cli.runs {
		if len(run) > 0 && run[0] == "run" {
			t.Errorf("kurtosis run was called despite invalid scenario")
		}
	}

	// No discovery file.
	_, readErr := discovery.Read()
	if !errors.Is(readErr, fs.ErrNotExist) {
		t.Errorf("expected no discovery file, got %v", readErr)
	}
}

func TestUp_MissingScenarioFlag(t *testing.T) {
	withDiscoveryDir(t)
	cli := &fakeCLI{}
	deps := &upDeps{cli: cli, httpClient: http.DefaultClient}
	_, _, err := runUpCmd(t, deps, "up")
	if err == nil {
		t.Fatal("expected error when --scenario not provided")
	}
}

// ── down tests ────────────────────────────────────────────────────────────────

func TestDown_FromDiscovery(t *testing.T) {
	withDiscoveryDir(t)

	// Seed the discovery file.
	if err := discovery.Write(&discovery.Current{
		EnclaveID:  "my-enc",
		ControlURL: "http://1.2.3.4:8090",
		Scenario:   "soak-test",
		StartedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	cli := &fakeCLI{}
	deps := &downDeps{cli: cli}

	stdout, _, err := runDownCmd(t, deps, "down", "--json")
	if err != nil {
		t.Fatalf("down: %v", err)
	}

	var got struct {
		EnclaveID string `json:"enclave_id"`
		OK        bool   `json:"ok"`
	}
	if jerr := json.Unmarshal([]byte(stdout), &got); jerr != nil {
		t.Fatalf("not JSON: %v (out=%q)", jerr, stdout)
	}
	if !got.OK {
		t.Errorf("expected ok:true, got %+v", got)
	}
	if got.EnclaveID != "my-enc" {
		t.Errorf("enclave_id: got %q want %q", got.EnclaveID, "my-enc")
	}

	// Discovery file must be removed.
	_, readErr := discovery.Read()
	if !errors.Is(readErr, fs.ErrNotExist) {
		t.Errorf("expected discovery file to be removed, got %v", readErr)
	}

	// CLI must have received enclave rm.
	var gotRM bool
	for _, run := range cli.runs {
		if len(run) >= 2 && run[0] == "enclave" && run[1] == "rm" {
			gotRM = true
			break
		}
	}
	if !gotRM {
		t.Errorf("expected enclave rm call; got: %v", cli.runs)
	}
}

func TestDown_PositionalArg(t *testing.T) {
	withDiscoveryDir(t)

	cli := &fakeCLI{}
	deps := &downDeps{cli: cli}

	stdout, _, err := runDownCmd(t, deps, "down", "--json", "my-named-enc")
	if err != nil {
		t.Fatalf("down: %v", err)
	}

	var got struct {
		EnclaveID string `json:"enclave_id"`
		OK        bool   `json:"ok"`
	}
	if jerr := json.Unmarshal([]byte(stdout), &got); jerr != nil {
		t.Fatalf("not JSON: %v (out=%q)", jerr, stdout)
	}
	if got.EnclaveID != "my-named-enc" {
		t.Errorf("enclave_id: got %q", got.EnclaveID)
	}
	if !got.OK {
		t.Errorf("expected ok:true")
	}
}

func TestDown_PersistentEnclaveFlag(t *testing.T) {
	// `confluence --enclave NAME down` must work without discovery — covers
	// the half-booted-enclave teardown path (Bug #5).
	withDiscoveryDir(t)

	cli := &fakeCLI{}
	deps := &downDeps{cli: cli}

	stdout, _, err := runDownCmd(t, deps, "--enclave", "half-booted-enc", "down", "--json")
	if err != nil {
		t.Fatalf("down: %v", err)
	}

	var got struct {
		EnclaveID string `json:"enclave_id"`
		OK        bool   `json:"ok"`
	}
	if jerr := json.Unmarshal([]byte(stdout), &got); jerr != nil {
		t.Fatalf("not JSON: %v (out=%q)", jerr, stdout)
	}
	if got.EnclaveID != "half-booted-enc" {
		t.Errorf("enclave_id: got %q want %q", got.EnclaveID, "half-booted-enc")
	}
	if !got.OK {
		t.Errorf("expected ok:true")
	}

	var gotRM bool
	for _, run := range cli.runs {
		if len(run) >= 4 && run[0] == "enclave" && run[1] == "rm" && run[3] == "half-booted-enc" {
			gotRM = true
			break
		}
	}
	if !gotRM {
		t.Errorf("expected enclave rm half-booted-enc; got: %v", cli.runs)
	}
}

func TestDown_NoEnclaveNoDiscovery(t *testing.T) {
	withDiscoveryDir(t)

	cli := &fakeCLI{}
	deps := &downDeps{cli: cli}

	_, _, err := runDownCmd(t, deps, "down")
	if err == nil {
		t.Fatal("expected error when no enclave and no discovery file")
	}
	if !strings.Contains(err.Error(), "no current enclave") {
		t.Errorf("error should mention 'no current enclave', got: %v", err)
	}

	// CLI must not have been called at all (besides possibly discovery).
	for _, run := range cli.runs {
		if len(run) >= 2 && run[0] == "enclave" && run[1] == "rm" {
			t.Errorf("enclave rm was called despite no enclave: %v", run)
		}
	}
}

func TestDown_Human(t *testing.T) {
	withDiscoveryDir(t)

	if err := discovery.Write(&discovery.Current{
		EnclaveID: "enc-human",
		StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	cli := &fakeCLI{}
	deps := &downDeps{cli: cli}

	stdout, _, err := runDownCmd(t, deps, "down")
	if err != nil {
		t.Fatalf("down: %v", err)
	}
	if !strings.Contains(stdout, "enc-human") {
		t.Errorf("expected enclave name in output, got %q", stdout)
	}
}
