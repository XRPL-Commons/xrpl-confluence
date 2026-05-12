package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// logsServer returns a test server that streams two NDJSON log lines.
func logsServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/logs", func(w http.ResponseWriter, r *http.Request) {
		node := r.URL.Query().Get("node")
		if node == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/x-ndjson")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ts":"2026-05-12T14:00:00.000Z","level":"info","node":"` + node + `","message":"started"}` + "\n"))
		w.Write([]byte(`{"ts":"2026-05-12T14:00:01.000Z","level":"warn","node":"` + node + `","message":"peer connected"}` + "\n"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestLogsJSON_HappyPath(t *testing.T) {
	srv := logsServer(t)

	outBuf, errBuf := &bytes.Buffer{}, &bytes.Buffer{}
	root := newRootCmd()
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	root.SetArgs([]string{"logs", "--node", "rippled-0", "--control-url", srv.URL})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("logs: %v", err)
	}

	output := outBuf.String()
	if !strings.Contains(output, "rippled-0") {
		t.Errorf("expected node name in output, got %q", output)
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		t.Errorf("expected at least 2 log lines, got %d", len(lines))
	}
}

func TestLogs_MissingNode(t *testing.T) {
	srv := logsServer(t)

	outBuf, errBuf := &bytes.Buffer{}, &bytes.Buffer{}
	root := newRootCmd()
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	root.SetArgs([]string{"logs", "--control-url", srv.URL})
	err := root.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected error when --node not provided")
	}
	if !strings.Contains(err.Error(), "--node") {
		t.Errorf("error should mention --node, got: %v", err)
	}
}
