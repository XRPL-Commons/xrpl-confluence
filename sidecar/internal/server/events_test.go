package server_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/finding"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/server"
)

func TestEvents_NoBus_Returns503(t *testing.T) {
	s := server.New()
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}

	var body api.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "events_unavailable" {
		t.Errorf("expected code %q, got %q", "events_unavailable", body.Error.Code)
	}
}

func TestEvents_StreamNodeAndFindingEvents(t *testing.T) {
	bus := server.NewEventBus()
	s := server.New(server.WithEventBus(bus))
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/v1/events", nil)
	if err != nil {
		t.Fatal(err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("unexpected Content-Type: %q", ct)
	}

	received := make(chan server.Event, 10)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			payload := strings.TrimPrefix(line, "data: ")
			var ev server.Event
			if err := json.Unmarshal([]byte(payload), &ev); err == nil {
				received <- ev
			}
		}
		close(received)
	}()

	// Give the stream a moment to establish.
	time.Sleep(50 * time.Millisecond)

	nodeEv := server.Event{
		Type:    "node",
		Payload: map[string]any{"nodes": []any{}},
		Ts:      time.Now().UnixMilli(),
	}
	bus.Publish(nodeEv)

	f := api.Finding{
		ID:       finding.NewFindingID(),
		Kind:     api.KindStateDivergence,
		Severity: api.SeverityError,
		Summary:  "test divergence",
		OpenedAt: time.Now(),
	}
	findingEv := server.Event{
		Type:    "finding",
		Payload: f,
		Ts:      time.Now().UnixMilli(),
	}
	bus.Publish(findingEv)

	got := make(map[string]bool)
	deadline := time.After(500 * time.Millisecond)
collect:
	for {
		select {
		case ev, ok := <-received:
			if !ok {
				break collect
			}
			got[ev.Type] = true
			if got["node"] && got["finding"] {
				break collect
			}
		case <-deadline:
			break collect
		}
	}

	if !got["node"] {
		t.Error("did not receive node event")
	}
	if !got["finding"] {
		t.Error("did not receive finding event")
	}
}

func TestEvents_ContextCancel_ExitsStream(t *testing.T) {
	bus := server.NewEventBus()
	s := server.New(server.WithEventBus(bus))
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/v1/events", nil)
	if err != nil {
		t.Fatal(err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Give the handler time to start.
	time.Sleep(50 * time.Millisecond)
	cancel()

	// The body read should unblock shortly after context cancellation.
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 64)
		for {
			_, err := resp.Body.Read(buf)
			if err != nil {
				close(done)
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Error("stream did not close after context cancel")
	}
}

func TestEventBus_SlowSubscriberDrops(t *testing.T) {
	bus := server.NewEventBus()

	// Subscribe with a tiny buffer.
	id, ch := bus.Subscribe(1)
	defer bus.Unsubscribe(id)

	// Publish more events than the buffer can hold without reading.
	for i := 0; i < 5; i++ {
		bus.Publish(server.Event{Type: "node", Ts: int64(i)})
	}

	// We should be able to drain the channel without blocking (drop happened).
	timeout := time.After(100 * time.Millisecond)
	drained := 0
drain:
	for {
		select {
		case <-ch:
			drained++
		case <-timeout:
			break drain
		}
	}

	if drained > 5 {
		t.Errorf("expected at most 5 events, got %d", drained)
	}
}
