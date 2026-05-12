package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/server"
)

type healthzResponse struct {
	OK              bool    `json:"ok"`
	APIVersion      string  `json:"api_version"`
	UptimeS         int     `json:"uptime_s"`
	Scenario        string  `json:"scenario"`
	BudgetRemaining *int    `json:"budget_remaining_s"`
}

func TestHealthz_Default(t *testing.T) {
	s := server.New()
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body healthzResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}

	if !body.OK {
		t.Error("expected ok=true")
	}
	if body.APIVersion != "confluence/v1" {
		t.Errorf("unexpected api_version: %q", body.APIVersion)
	}
	if body.UptimeS < 0 {
		t.Errorf("unexpected uptime_s: %d", body.UptimeS)
	}
	if body.Scenario != "" {
		t.Errorf("expected empty scenario, got %q", body.Scenario)
	}
	if body.BudgetRemaining != nil {
		t.Errorf("expected no budget_remaining_s, got %d", *body.BudgetRemaining)
	}
}

func TestHealthz_WithScenarioAndBudget(t *testing.T) {
	deadline := time.Now().Add(30 * time.Second)
	s := server.New(server.WithScenario("soak-mixed-3x2"), server.WithBudget(deadline))
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var body healthzResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}

	if body.Scenario != "soak-mixed-3x2" {
		t.Errorf("unexpected scenario: %q", body.Scenario)
	}
	if body.BudgetRemaining == nil {
		t.Fatal("expected budget_remaining_s to be present")
	}
	if *body.BudgetRemaining < 28 || *body.BudgetRemaining > 30 {
		t.Errorf("budget_remaining_s %d not in [28,30]", *body.BudgetRemaining)
	}
}

func TestHealthz_PastDeadline(t *testing.T) {
	past := time.Now().Add(-10 * time.Second)
	s := server.New(server.WithBudget(past))
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var body healthzResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}

	if body.BudgetRemaining == nil {
		t.Fatal("expected budget_remaining_s to be present")
	}
	if *body.BudgetRemaining != 0 {
		t.Errorf("expected budget_remaining_s=0, got %d", *body.BudgetRemaining)
	}
}
