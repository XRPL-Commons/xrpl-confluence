package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/finding"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/server"
)

// validScenarioBody returns a minimal valid scenario JSON for use in POST /v1/runs.
func validScenarioBody(budget string, stopOn []string) string {
	sc := api.Scenario{
		APIVersion: "confluence/v1",
		Kind:       "Scenario",
		Metadata:   api.ScenarioMetadata{Name: "test-run"},
		Topology: api.Topology{
			Rippled: api.NodeGroup{Count: 1},
			Goxrpl:  api.NodeGroup{Count: 1},
		},
		Workload: api.Workload{Kind: api.WorkloadSoak},
		Budget:   api.Budget{Duration: budget, StopOn: stopOn},
	}
	req := server.StartRunRequest{Scenario: sc}
	b, _ := json.Marshal(req)
	return string(b)
}

func newRunsServer(store *finding.Store, bus *server.EventBus) (*server.Server, *httptest.Server) {
	opts := []server.Option{}
	if store != nil {
		opts = append(opts, server.WithFindingStore(store))
	}
	if bus != nil {
		opts = append(opts, server.WithEventBus(bus))
	}
	srv := server.New(opts...)
	ts := httptest.NewServer(srv.Handler())
	return srv, ts
}

func TestRuns_StartRun_Valid(t *testing.T) {
	store := finding.NewStore()
	bus := server.NewEventBus()
	_, ts := newRunsServer(store, bus)
	defer ts.Close()

	body := validScenarioBody("50ms", nil)
	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var out server.StartRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Run.ID == "" {
		t.Fatal("run ID is empty")
	}
	if out.Run.Status != server.RunStatusRunning {
		t.Fatalf("expected status running, got %q", out.Run.Status)
	}
	if out.Run.Scenario != "test-run" {
		t.Fatalf("unexpected scenario name: %q", out.Run.Scenario)
	}

	runID := out.Run.ID

	// Wait for budget to elapse and run to transition to completed_budget.
	deadline := time.Now().Add(2 * time.Second)
	var gotStatus string
	for time.Now().Before(deadline) {
		r := getJSON(t, ts.URL+"/v1/runs/"+runID)
		gotStatus, _ = r["status"].(string)
		if gotStatus == server.RunStatusCompletedBudget {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if gotStatus != server.RunStatusCompletedBudget {
		t.Fatalf("expected status %q after budget, got %q", server.RunStatusCompletedBudget, gotStatus)
	}
}

func TestRuns_StartRun_InvalidScenario(t *testing.T) {
	_, ts := newRunsServer(nil, nil)
	defer ts.Close()

	// Send a scenario missing required fields.
	body := `{"scenario":{"api_version":"confluence/v1","kind":"Scenario","metadata":{"name":""},"topology":{"rippled":{"count":0},"goxrpl":{"count":0}},"workload":{"kind":"soak"},"budget":{"duration":"1m"}}}`
	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var errResp api.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatal(err)
	}
	if errResp.Error.Code != api.ErrCodeScenarioInvalid {
		t.Fatalf("expected code %q, got %q", api.ErrCodeScenarioInvalid, errResp.Error.Code)
	}
}

func TestRuns_StopOn_FirstDivergence(t *testing.T) {
	store := finding.NewStore()
	bus := server.NewEventBus()

	// Wire bus to publish on finding add.
	store.SetOnAdd(func(f api.Finding) {
		bus.Publish(server.Event{Type: "finding", Payload: f, Ts: f.OpenedAt.UnixMilli()})
	})

	_, ts := newRunsServer(store, bus)
	defer ts.Close()

	body := validScenarioBody("10m", []string{api.StopOnFirstDivergence})
	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var out server.StartRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	runID := out.Run.ID

	// Add a state_divergence finding — should trigger stop_on.
	f := api.Finding{
		ID:       finding.NewFindingID(),
		Kind:     api.KindStateDivergence,
		OpenedAt: time.Now(),
		Summary:  "divergence triggered",
	}
	store.Add(f)

	// Within 1s the run should transition to completed_stop_on.
	deadline := time.Now().Add(1 * time.Second)
	var gotStatus string
	var triggerFinding string
	for time.Now().Before(deadline) {
		r := getJSON(t, ts.URL+"/v1/runs/"+runID)
		gotStatus, _ = r["status"].(string)
		triggerFinding, _ = r["trigger_finding"].(string)
		if gotStatus == server.RunStatusCompletedStopOn {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}

	if gotStatus != server.RunStatusCompletedStopOn {
		t.Fatalf("expected status %q, got %q", server.RunStatusCompletedStopOn, gotStatus)
	}
	if triggerFinding != f.ID {
		t.Fatalf("expected trigger_finding %q, got %q", f.ID, triggerFinding)
	}
}

func TestRuns_RunByID_NotFound(t *testing.T) {
	_, ts := newRunsServer(nil, nil)
	defer ts.Close()

	resp := getJSONStatus(t, ts.URL+"/v1/runs/run_nonexistent", http.StatusNotFound)
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("error field not an object: %v", resp)
	}
	if errObj["code"] != api.ErrCodeRunNotFound {
		t.Fatalf("expected code %q, got %q", api.ErrCodeRunNotFound, errObj["code"])
	}
}

func TestRuns_ListRuns(t *testing.T) {
	store := finding.NewStore()
	bus := server.NewEventBus()
	_, ts := newRunsServer(store, bus)
	defer ts.Close()

	// Start a run.
	body := validScenarioBody("1m", nil)
	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	list := getJSON(t, ts.URL+"/v1/runs")
	runs, ok := list["runs"].([]any)
	if !ok {
		t.Fatalf("runs is not an array: %T", list["runs"])
	}
	if len(runs) == 0 {
		t.Fatal("expected at least one run")
	}
}
