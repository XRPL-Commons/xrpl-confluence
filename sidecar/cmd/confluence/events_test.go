package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/server"
)

// eventsServer boots a real server with an EventBus, publishes events, and
// returns a test server + a function to publish events into the bus.
func eventsServer(t *testing.T) (*httptest.Server, *server.EventBus) {
	t.Helper()
	bus := server.NewEventBus()
	s := server.New(server.WithEventBus(bus))
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv, bus
}

func TestEvents_NDJSON_HappyPath(t *testing.T) {
	srv, bus := eventsServer(t)

	outBuf := &bytes.Buffer{}
	root := newRootCmd()
	root.SetOut(outBuf)
	root.SetErr(&bytes.Buffer{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		root.SetArgs([]string{"events", "--control-url", srv.URL})
		done <- root.ExecuteContext(ctx)
	}()

	// Give the stream time to connect.
	time.Sleep(80 * time.Millisecond)

	bus.Publish(server.Event{Type: "node", Payload: map[string]any{"nodes": []any{}}, Ts: 1})
	bus.Publish(server.Event{Type: "finding", Payload: map[string]any{"id": "f1"}, Ts: 2})

	// Wait until we see both events in the output buffer.
	deadline := time.After(500 * time.Millisecond)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for both events in output")
		default:
		}
		out := outBuf.String()
		if strings.Contains(out, `"node"`) && strings.Contains(out, `"finding"`) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("events cmd: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("command did not exit after context cancel")
	}

	// Verify each output line is valid JSON.
	for _, line := range strings.Split(strings.TrimSpace(outBuf.String()), "\n") {
		if line == "" {
			continue
		}
		var v map[string]any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Errorf("line is not valid JSON: %q", line)
		}
	}
}

func TestStreamSSEAsNDJSON_SkipsCommentsAndBlanks(t *testing.T) {
	input := ":ok\n\ndata: {\"type\":\"node\"}\n\n:keepalive\n\ndata: {\"type\":\"finding\"}\n\n"
	out := &bytes.Buffer{}
	if err := streamSSEAsNDJSON(strings.NewReader(input), out); err != nil {
		t.Fatalf("streamSSEAsNDJSON: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 NDJSON lines, got %d: %q", len(lines), out.String())
	}
	if lines[0] != `{"type":"node"}` {
		t.Errorf("line 0: got %q", lines[0])
	}
	if lines[1] != `{"type":"finding"}` {
		t.Errorf("line 1: got %q", lines[1])
	}
}

func TestEvents_NoControlURL_Errors(t *testing.T) {
	// Use a port that is definitely not listening.
	outBuf, errBuf := &bytes.Buffer{}, &bytes.Buffer{}
	root := newRootCmd()
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	root.SetArgs([]string{"events", "--control-url", "http://127.0.0.1:1"})

	err := root.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected error when control service is unreachable")
	}
}

func TestEvents_SSEServer_Serves(t *testing.T) {
	// A simple hand-rolled SSE server to verify the adapter without needing
	// the full server package.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(":ok\n\n"))
		w.Write([]byte("data: {\"type\":\"ping\"}\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	outBuf := &bytes.Buffer{}
	root := newRootCmd()
	root.SetOut(outBuf)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"events", "--control-url", srv.URL})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := root.ExecuteContext(ctx); err != nil {
		t.Fatalf("events: %v", err)
	}

	out := strings.TrimSpace(outBuf.String())
	if out != `{"type":"ping"}` {
		t.Errorf("unexpected output: %q", out)
	}
}
