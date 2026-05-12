package finding

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
)

func TestMapDivergence_KindMapping(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		corpusKind string
		wantKind   string
	}{
		{"state_hash", api.KindStateDivergence},
		{"tx_result", api.KindStateDivergence},
		{"metadata", api.KindStateDivergence},
		{"invariant", api.KindChaosViolation},
		{"crash", api.KindNodeCrash},
		{"garbage", api.KindStateDivergence}, // unknown → safe default
	}
	for _, tc := range cases {
		t.Run(tc.corpusKind, func(t *testing.T) {
			d := corpus.Divergence{
				Kind:        tc.corpusKind,
				Description: "test desc",
				RecordedAt:  now,
			}
			f, err := MapDivergence(d)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if f.Kind != tc.wantKind {
				t.Errorf("kind: got %q, want %q", f.Kind, tc.wantKind)
			}
			if !strings.HasPrefix(f.ID, "fnd_") {
				t.Errorf("bad ID: %q", f.ID)
			}
			if f.Severity != api.SeverityError {
				t.Errorf("severity: got %q, want %q", f.Severity, api.SeverityError)
			}
			if f.Summary != d.Description {
				t.Errorf("summary: got %q, want %q", f.Summary, d.Description)
			}
			if !f.OpenedAt.Equal(now) {
				t.Errorf("opened_at: got %v, want %v", f.OpenedAt, now)
			}
		})
	}
}

func TestMapDivergence_Details(t *testing.T) {
	d := corpus.Divergence{
		Kind:       "state_hash",
		Details:    map[string]any{"foo": "bar", "num": float64(42)},
		RecordedAt: time.Now().UTC(),
	}
	f, err := MapDivergence(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Detail == nil {
		t.Fatal("expected non-nil Detail")
	}
	var got map[string]any
	if err := json.Unmarshal(f.Detail, &got); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}
	if got["foo"] != "bar" {
		t.Errorf("detail[foo]: got %v", got["foo"])
	}
}

func TestMapDivergence_NilDetails(t *testing.T) {
	d := corpus.Divergence{
		Kind:       "crash",
		RecordedAt: time.Now().UTC(),
	}
	f, err := MapDivergence(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Detail != nil {
		t.Errorf("expected nil Detail, got %s", f.Detail)
	}
}
