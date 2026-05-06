package alert

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestWebhook_DedupBySignature(t *testing.T) {
	var posts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&posts, 1)
		var got map[string]any
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		if got["text"] == "" {
			t.Errorf("no text field in payload: %s", body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wh := NewWebhook(srv.URL)
	wh.Maybe("sig-A", "first A divergence")
	wh.Maybe("sig-A", "second A divergence (dup)")
	wh.Maybe("sig-B", "first B divergence")

	wh.Wait()
	if got := atomic.LoadInt32(&posts); got != 2 {
		t.Errorf("want 2 posts (sig-A first + sig-B first), got %d", got)
	}
}

func TestWebhook_AlwaysFiresWhenSignatureEmpty(t *testing.T) {
	var posts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&posts, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wh := NewWebhook(srv.URL)
	wh.Maybe("", "crash 1")
	wh.Maybe("", "crash 2")
	wh.Wait()

	if got := atomic.LoadInt32(&posts); got != 2 {
		t.Errorf("want 2 posts (no dedup when signature empty), got %d", got)
	}
}

func TestWebhook_NilSafe(t *testing.T) {
	var wh *Webhook
	wh.Maybe("sig", "anything")
	wh.Wait()
}

func TestNewWebhook_EmptyURLReturnsNil(t *testing.T) {
	if wh := NewWebhook(""); wh != nil {
		t.Errorf("want nil on empty URL, got %#v", wh)
	}
}
