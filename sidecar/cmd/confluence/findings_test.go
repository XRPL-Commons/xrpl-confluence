package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// findingsServer returns a test server with /v1/findings and /v1/findings/{id}.
func findingsServer(t *testing.T) *httptest.Server {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/findings", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"id":"fnd_abc123","kind":"state_divergence","opened_at":"` + now + `","summary":"nodes diverged"}]`))
	})
	mux.HandleFunc("GET /v1/findings/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "fnd_abc123" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id":"fnd_abc123","kind":"state_divergence","opened_at":"` + now + `","summary":"nodes diverged"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":{"code":"finding_not_found","message":"finding not found"}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestFindingsJSON_HappyPath(t *testing.T) {
	srv := findingsServer(t)

	outBuf, errBuf := &bytes.Buffer{}, &bytes.Buffer{}
	root := newRootCmd()
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	root.SetArgs([]string{"findings", "--json", "--control-url", srv.URL})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("findings: %v", err)
	}

	var got []map[string]any
	if jerr := json.Unmarshal(outBuf.Bytes(), &got); jerr != nil {
		t.Fatalf("not JSON: %v (out=%q)", jerr, outBuf.String())
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(got))
	}
	if got[0]["id"] != "fnd_abc123" {
		t.Errorf("id: got %v", got[0]["id"])
	}
}

func TestFindingShow_NotFound(t *testing.T) {
	srv := findingsServer(t)

	outBuf, errBuf := &bytes.Buffer{}, &bytes.Buffer{}
	root := newRootCmd()
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	root.SetArgs([]string{"finding", "show", "fnd_nonexistent", "--json", "--control-url", srv.URL})
	err := root.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected non-nil error for 404")
	}

	var got map[string]any
	if jerr := json.Unmarshal(outBuf.Bytes(), &got); jerr != nil {
		t.Fatalf("not JSON: %v (out=%q)", jerr, outBuf.String())
	}
	errObj, ok := got["error"].(map[string]any)
	if !ok {
		t.Fatalf("error not an object: %v", got["error"])
	}
	if errObj["code"] != "finding_not_found" {
		t.Errorf("code: got %v", errObj["code"])
	}
}
