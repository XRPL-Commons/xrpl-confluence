package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/finding"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/server"
)

func makeTestFindings() (store *finding.Store, f1, f2, f3 api.Finding) {
	store = finding.NewStore()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	f1 = api.Finding{
		ID:       finding.NewFindingID(),
		Kind:     api.KindStateDivergence,
		Severity: api.SeverityError,
		OpenedAt: base.Add(1 * time.Second),
		Summary:  "oldest",
	}
	f2 = api.Finding{
		ID:       finding.NewFindingID(),
		Kind:     api.KindNodeCrash,
		Severity: api.SeverityError,
		OpenedAt: base.Add(2 * time.Second),
		Summary:  "middle",
	}
	f3 = api.Finding{
		ID:       finding.NewFindingID(),
		Kind:     api.KindStateDivergence,
		Severity: api.SeverityError,
		OpenedAt: base.Add(3 * time.Second),
		Summary:  "newest",
	}

	store.Add(f1)
	store.Add(f2)
	store.Add(f3)

	return store, f1, f2, f3
}

func TestFindings_NoStore_List(t *testing.T) {
	s := server.New()
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/findings")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var got []api.Finding
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty array, got %d items", len(got))
	}
}

func TestFindings_NoStore_GetByID(t *testing.T) {
	s := server.New()
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/findings/fnd_xxx")
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
	if body.Error.Code != api.ErrCodeFindingNotFound {
		t.Errorf("expected code %q, got %q", api.ErrCodeFindingNotFound, body.Error.Code)
	}
}

func TestFindings_List_All(t *testing.T) {
	store, _, _, f3 := makeTestFindings()
	s := server.New(server.WithFindingStore(store))
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/findings")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var got []api.Finding
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 findings, got %d", len(got))
	}
	// newest-first
	if got[0].ID != f3.ID {
		t.Errorf("expected newest first, got %q", got[0].ID)
	}
}

func TestFindings_List_Limit(t *testing.T) {
	store, _, _, _ := makeTestFindings()
	s := server.New(server.WithFindingStore(store))
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/findings?limit=1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var got []api.Finding
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(got))
	}
}

func TestFindings_List_KindFilter(t *testing.T) {
	store, f1, _, f3 := makeTestFindings()
	s := server.New(server.WithFindingStore(store))
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/findings?kind=state_divergence")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var got []api.Finding
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 state_divergence findings, got %d", len(got))
	}
	ids := map[string]bool{got[0].ID: true, got[1].ID: true}
	if !ids[f1.ID] || !ids[f3.ID] {
		t.Errorf("unexpected findings: %v", got)
	}
}

func TestFindings_List_SinceFilter(t *testing.T) {
	store, _, f2, f3 := makeTestFindings()
	s := server.New(server.WithFindingStore(store))
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/findings?since=" + f2.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var got []api.Finding
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != f3.ID {
		t.Errorf("expected only f3, got %v", got)
	}
}

func TestFindings_GetByID_Found(t *testing.T) {
	store, f1, _, _ := makeTestFindings()
	s := server.New(server.WithFindingStore(store))
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/findings/" + f1.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var got api.Finding
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.ID != f1.ID {
		t.Errorf("expected ID %q, got %q", f1.ID, got.ID)
	}
}

func TestFindings_GetByID_NotFound(t *testing.T) {
	store, _, _, _ := makeTestFindings()
	s := server.New(server.WithFindingStore(store))
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/findings/fnd_nonexistent")
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
	if body.Error.Code != api.ErrCodeFindingNotFound {
		t.Errorf("expected code %q, got %q", api.ErrCodeFindingNotFound, body.Error.Code)
	}
}

func TestFindings_List_BadLimit(t *testing.T) {
	store, _, _, _ := makeTestFindings()
	s := server.New(server.WithFindingStore(store))
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/findings?limit=abc")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var body api.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "bad_request" {
		t.Errorf("expected code %q, got %q", "bad_request", body.Error.Code)
	}
}
