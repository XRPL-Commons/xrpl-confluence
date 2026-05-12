package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// statusServer returns an httptest.Server with minimal /v1/healthz, /v1/nodes,
// /v1/findings, and /v1/state/diff endpoints for status tests.
func statusServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"api_version":"confluence/v1","uptime_s":30,"scenario":"test-scenario"}`))
	})
	mux.HandleFunc("/v1/nodes", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"timestamp":1000,"nodes":[{"name":"rippled-0","type":"rippled","status":"ok","peers":3}]}`))
	})
	mux.HandleFunc("/v1/findings", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	})
	mux.HandleFunc("/v1/state/diff", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ledger":100,"hash_by_node":{},"diverged":false,"as_of":1000}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestStatusJSON_HappyPath(t *testing.T) {
	srv := statusServer(t)

	outBuf, errBuf := &bytes.Buffer{}, &bytes.Buffer{}
	root := newRootCmd()
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	root.SetArgs([]string{"status", "--json", "--control-url", srv.URL})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("status: %v", err)
	}

	var got map[string]any
	if jerr := json.Unmarshal(outBuf.Bytes(), &got); jerr != nil {
		t.Fatalf("not JSON: %v (out=%q)", jerr, outBuf.String())
	}

	if _, ok := got["healthz"]; !ok {
		t.Error("missing healthz key")
	}
	if _, ok := got["nodes"]; !ok {
		t.Error("missing nodes key")
	}
	if _, ok := got["state_diff"]; !ok {
		t.Error("missing state_diff key")
	}
	if _, ok := got["latest_finding"]; !ok {
		t.Error("missing latest_finding key")
	}
}

func TestStatus_MissingControlURL(t *testing.T) {
	withDiscoveryDir(t)

	outBuf, errBuf := &bytes.Buffer{}, &bytes.Buffer{}
	root := newRootCmd()
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	root.SetArgs([]string{"status"})
	err := root.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected error when no discovery and no --control-url")
	}
}
