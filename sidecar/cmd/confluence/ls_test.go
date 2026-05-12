package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// lsHealthzServer returns a test server serving /v1/healthz with the given scenario.
func lsHealthzServer(t *testing.T, scenario string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true,"api_version":"confluence/v1","uptime_s":60,"scenario":"`+scenario+`"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// fakeCLIForLsWithServer returns a fakeCLI that lists one enclave and returns
// the given host (without port) for service inspect, plus a redirectClient
// pointed at srv so http://<host>:8090 reaches it.
func fakeCLIForLsWithServer(enclaveName, host string) *fakeCLI {
	lsOut := "Name\tStatus\n" + enclaveName + "\tRUNNING\n"
	inspectOut := "UUID: test-uuid\nIP Address: " + host + "\n"
	return &fakeCLI{
		next: func(args []string) (string, string, error) {
			if len(args) >= 2 && args[0] == "enclave" && args[1] == "ls" {
				return lsOut, "", nil
			}
			if len(args) >= 2 && args[0] == "service" && args[1] == "inspect" {
				return inspectOut, "", nil
			}
			return "", "", nil
		},
	}
}

// runLsCmdWith runs the ls command with injected lsDeps.
func runLsCmdWith(t *testing.T, deps *lsDeps, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	outBuf, errBuf := &bytes.Buffer{}, &bytes.Buffer{}
	root := newRootCmd()
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	replaceSubCmd(root, newLsCmdWith(deps))
	root.SetArgs(args)
	err = root.ExecuteContext(context.Background())
	return outBuf.String(), errBuf.String(), err
}

func TestLsJSON_HappyPath(t *testing.T) {
	srv := lsHealthzServer(t, "soak-test")

	// Extract host (without port) for the IP Address field.
	hostPort := strings.TrimPrefix(srv.URL, "http://")
	host := hostPort
	if idx := strings.LastIndex(hostPort, ":"); idx >= 0 {
		host = hostPort[:idx]
	}

	cli := fakeCLIForLsWithServer("my-enclave", host)
	deps := &lsDeps{
		cli:        cli,
		httpClient: redirectClient(srv),
	}

	stdout, _, err := runLsCmdWith(t, deps, "ls", "--json")
	if err != nil {
		t.Fatalf("ls: %v", err)
	}

	var rows []map[string]any
	if jerr := json.Unmarshal([]byte(stdout), &rows); jerr != nil {
		t.Fatalf("not JSON: %v (out=%q)", jerr, stdout)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	row := rows[0]
	if row["enclave_id"] != "my-enclave" {
		t.Errorf("enclave_id: got %v", row["enclave_id"])
	}
	if row["status"] != "ok" {
		t.Errorf("status: got %v", row["status"])
	}
	if row["scenario"] != "soak-test" {
		t.Errorf("scenario: got %v", row["scenario"])
	}
}

func TestLs_UnhealthyEnclave(t *testing.T) {
	// No real server; service inspect fails → unhealthy.
	cli := &fakeCLI{
		next: func(args []string) (string, string, error) {
			if len(args) >= 2 && args[0] == "enclave" && args[1] == "ls" {
				return "Name\tStatus\nbad-enc\tRUNNING\n", "", nil
			}
			if len(args) >= 2 && args[0] == "service" && args[1] == "inspect" {
				return "", "error: not found", fmt.Errorf("exit 1")
			}
			return "", "", nil
		},
	}
	deps := &lsDeps{cli: cli}

	stdout, _, _ := runLsCmdWith(t, deps, "ls", "--json")

	var rows []map[string]any
	if jerr := json.Unmarshal([]byte(stdout), &rows); jerr != nil {
		t.Fatalf("not JSON: %v (out=%q)", jerr, stdout)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if status, ok := rows[0]["status"].(string); !ok || !strings.HasPrefix(status, "unhealthy") {
		t.Errorf("status: expected unhealthy prefix, got %v", rows[0]["status"])
	}
}
