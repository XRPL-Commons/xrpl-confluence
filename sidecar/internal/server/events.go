package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
)

// Event is a single SSE payload published on the event bus.
type Event struct {
	Type    string `json:"type"`    // "node" | "finding"
	Payload any    `json:"payload"`
	Ts      int64  `json:"ts"` // unix millis
}

// EventBus is a simple in-memory fan-out. Subscribers receive on a
// per-subscriber buffered channel; a slow subscriber drops events
// rather than blocking publishers.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[int]chan Event
	nextID      int
}

// NewEventBus returns an initialised EventBus.
func NewEventBus() *EventBus {
	return &EventBus{subscribers: make(map[int]chan Event)}
}

// Publish sends ev to all current subscribers. Slow subscribers (full buffer)
// drop the event rather than blocking the publisher.
func (b *EventBus) Publish(ev Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers {
		select {
		case ch <- ev:
		default:
		}
	}
}

// Subscribe registers a new subscriber with the given channel buffer size and
// returns its id and receive-only channel.
func (b *EventBus) Subscribe(buf int) (id int, ch <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id = b.nextID
	b.nextID++
	c := make(chan Event, buf)
	b.subscribers[id] = c
	return id, c
}

// Unsubscribe removes the subscriber and closes its channel.
func (b *EventBus) Unsubscribe(id int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if c, ok := b.subscribers[id]; ok {
		delete(b.subscribers, id)
		close(c)
	}
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	if s.eventBus == nil {
		writeJSON(w, http.StatusServiceUnavailable, api.ErrorResponse{
			Error: api.Error{Code: "events_unavailable", Message: "event bus not configured"},
		})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, api.ErrorResponse{
			Error: api.Error{Code: "internal_error", Message: "streaming not supported"},
		})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	// Initial keepalive so the client knows the stream is live.
	fmt.Fprint(w, ":ok\n\n")
	flusher.Flush()

	id, ch := s.eventBus.Subscribe(32)
	defer s.eventBus.Unsubscribe(id)

	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			fmt.Fprint(w, ":keepalive\n\n")
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			b, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
	}
}
