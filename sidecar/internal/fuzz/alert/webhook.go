// Package alert posts notifications to a Slack/Discord-compatible webhook
// when first-seen divergences or crashes occur. In-process dedup keyed by
// signature prevents pager spam; an empty signature bypasses dedup so every
// crash fires.
package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

// Webhook posts JSON {"text": "..."} bodies to a Slack/Discord-compatible
// incoming webhook URL. Posts run in background goroutines so the caller
// (a fuzz loop) is never blocked by network latency. Wait blocks until all
// in-flight posts complete — call it before sidecar shutdown if completeness
// matters; for steady-state runs it can be skipped.
type Webhook struct {
	url    string
	client *http.Client

	mu   sync.Mutex
	seen map[string]bool
	wg   sync.WaitGroup
}

// NewWebhook returns nil when url is empty so callers can ignore it.
func NewWebhook(url string) *Webhook {
	if url == "" {
		return nil
	}
	return &Webhook{
		url:    url,
		client: &http.Client{Timeout: 10 * time.Second},
		seen:   map[string]bool{},
	}
}

// Maybe posts text to the webhook unless this signature has already fired
// once. An empty signature bypasses dedup (used for crashes — every crash
// is interesting). Safe to call on a nil receiver.
func (w *Webhook) Maybe(signature, text string) {
	if w == nil {
		return
	}
	if signature != "" {
		w.mu.Lock()
		if w.seen[signature] {
			w.mu.Unlock()
			return
		}
		w.seen[signature] = true
		w.mu.Unlock()
	}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.post(text)
	}()
}

// Wait blocks until all in-flight posts have completed. Safe on nil.
func (w *Webhook) Wait() {
	if w == nil {
		return
	}
	w.wg.Wait()
}

func (w *Webhook) post(text string) {
	body, _ := json.Marshal(map[string]string{"text": text})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		log.Printf("alert: build request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.client.Do(req)
	if err != nil {
		log.Printf("alert: post: %v", err)
		return
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.Printf("alert: webhook returned %s", resp.Status)
	}
}
