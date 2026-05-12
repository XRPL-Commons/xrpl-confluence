package server_test

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/server"
)

type logLine struct {
	Ts      string `json:"ts"`
	Level   string `json:"level,omitempty"`
	Node    string `json:"node"`
	Message string `json:"message"`
}

func writeLogFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func decodeNDJSON(t *testing.T, resp *http.Response) []logLine {
	t.Helper()
	var lines []logLine
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		raw := sc.Bytes()
		if len(raw) == 0 {
			continue
		}
		var l logLine
		if err := json.Unmarshal(raw, &l); err != nil {
			t.Fatalf("invalid NDJSON line %q: %v", raw, err)
		}
		lines = append(lines, l)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return lines
}

const logContent = "" +
	"2026-05-12T14:00:00.000Z [info] hello world\n" +
	"2026-05-12T14:00:01.000Z [warn] something bad\n" +
	"raw line without timestamp\n"

func TestLogs_All(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "rippled-0.log", logContent)

	s := server.New(server.WithLogsDir(dir))
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/logs?node=rippled-0")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/x-ndjson" {
		t.Errorf("expected Content-Type text/x-ndjson, got %q", ct)
	}

	lines := decodeNDJSON(t, resp)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	if lines[0].Node != "rippled-0" {
		t.Errorf("expected node rippled-0, got %q", lines[0].Node)
	}
	if lines[0].Ts != "2026-05-12T14:00:00.000Z" {
		t.Errorf("unexpected ts: %q", lines[0].Ts)
	}
	if lines[0].Level != "info" {
		t.Errorf("expected level info, got %q", lines[0].Level)
	}
	if lines[0].Message != "hello world" {
		t.Errorf("unexpected message: %q", lines[0].Message)
	}

	if lines[1].Level != "warn" {
		t.Errorf("expected level warn, got %q", lines[1].Level)
	}

	// raw line: no parseable timestamp → Ts is now, Level is empty, Message is full raw line
	if lines[2].Level != "" {
		t.Errorf("expected empty level for raw line, got %q", lines[2].Level)
	}
	if lines[2].Message != "raw line without timestamp" {
		t.Errorf("unexpected raw message: %q", lines[2].Message)
	}
}

func TestLogs_NotFound(t *testing.T) {
	dir := t.TempDir()

	s := server.New(server.WithLogsDir(dir))
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/logs?node=missing")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}

	var body api.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != api.ErrCodeLogsNotFound {
		t.Errorf("expected code %q, got %q", api.ErrCodeLogsNotFound, body.Error.Code)
	}
	if body.Error.Field != "node" {
		t.Errorf("expected field node, got %q", body.Error.Field)
	}
}

func TestLogs_BadNodeParam(t *testing.T) {
	dir := t.TempDir()

	s := server.New(server.WithLogsDir(dir))
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	for _, bad := range []string{"BAD!chars", "", "has space", "../escape"} {
		resp, err := http.Get(ts.URL + "/v1/logs?node=" + bad)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("node=%q: expected 400, got %d", bad, resp.StatusCode)
		}
	}
}

func TestLogs_GrepFilter(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "rippled-0.log", logContent)

	s := server.New(server.WithLogsDir(dir))
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/logs?node=rippled-0&grep=hello")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	lines := decodeNDJSON(t, resp)
	if len(lines) != 1 {
		t.Fatalf("expected 1 matching line, got %d", len(lines))
	}
	if lines[0].Message != "hello world" {
		t.Errorf("unexpected message: %q", lines[0].Message)
	}
}

func TestLogs_Limit(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "rippled-0.log", logContent)

	s := server.New(server.WithLogsDir(dir))
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/logs?node=rippled-0&limit=2")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	lines := decodeNDJSON(t, resp)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (limit=2), got %d", len(lines))
	}
}
