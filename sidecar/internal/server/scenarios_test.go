package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/server"
)

const validScenarioYAML = `apiVersion: confluence/v1
kind: Scenario
metadata:
  name: soak-basic
  description: basic soak test
topology:
  rippled:
    count: 2
  goxrpl:
    count: 1
workload:
  kind: soak
budget:
  duration: 5m
`

const validScenario2YAML = `apiVersion: confluence/v1
kind: Scenario
metadata:
  name: fuzz-chaos
  description: fuzz with chaos schedule
topology:
  rippled:
    count: 1
  goxrpl:
    count: 1
workload:
  kind: fuzz
chaos:
  schedule:
    - step: 10
      type: kill_container
      container: rippled-0
budget:
  duration: 2m
`

const malformedYAML = `apiVersion: confluence/v1
kind: Scenario
metadata:
  name: [bad yaml
`

func makeScenariosDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "scenario1.yaml"), []byte(validScenarioYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scenario2.yaml"), []byte(validScenario2YAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte(malformedYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

type scenarioListResponse struct {
	Scenarios []scenarioListItem `json:"scenarios"`
}

type scenarioListItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
}

type validateResponse struct {
	OK     bool        `json:"ok"`
	Errors []api.Error `json:"errors"`
}

func TestScenarios_List_WithDir(t *testing.T) {
	dir := makeScenariosDir(t)
	s := server.New(server.WithScenariosDir(dir))
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/scenarios")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body scenarioListResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}

	if len(body.Scenarios) != 2 {
		t.Fatalf("expected 2 scenarios (malformed skipped), got %d", len(body.Scenarios))
	}

	names := map[string]bool{}
	for _, sc := range body.Scenarios {
		names[sc.Name] = true
		if sc.Path == "" {
			t.Errorf("scenario %q has empty path", sc.Name)
		}
	}
	if !names["soak-basic"] {
		t.Error("expected soak-basic in list")
	}
	if !names["fuzz-chaos"] {
		t.Error("expected fuzz-chaos in list")
	}
}

func TestScenarios_List_NoDir(t *testing.T) {
	s := server.New()
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/scenarios")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body scenarioListResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}

	if len(body.Scenarios) != 0 {
		t.Fatalf("expected empty scenarios, got %d", len(body.Scenarios))
	}
}

func TestScenarios_Validate_Valid(t *testing.T) {
	s := server.New()
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	sc := api.Scenario{
		APIVersion: "confluence/v1",
		Kind:       "Scenario",
		Metadata:   api.ScenarioMetadata{Name: "my-test", Description: "desc"},
		Topology:   api.Topology{Rippled: api.NodeGroup{Count: 1}},
		Workload:   api.Workload{Kind: "soak"},
		Budget:     api.Budget{Duration: "10m"},
	}
	body, _ := json.Marshal(sc)

	resp, err := http.Post(ts.URL+"/v1/scenarios/validate", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var got validateResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !got.OK {
		t.Errorf("expected ok=true, got errors: %v", got.Errors)
	}
	if len(got.Errors) != 0 {
		t.Errorf("expected no errors, got %v", got.Errors)
	}
}

func TestScenarios_Validate_Invalid(t *testing.T) {
	s := server.New()
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	sc := api.Scenario{
		APIVersion: "bad/version",
		Kind:       "Scenario",
		Metadata:   api.ScenarioMetadata{Name: "my-test"},
		Topology:   api.Topology{Rippled: api.NodeGroup{Count: 1}},
		Workload:   api.Workload{Kind: "soak"},
		Budget:     api.Budget{Duration: "10m"},
	}
	body, _ := json.Marshal(sc)

	resp, err := http.Post(ts.URL+"/v1/scenarios/validate", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 (ok=false in body), got %d", resp.StatusCode)
	}

	var got validateResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.OK {
		t.Error("expected ok=false for invalid scenario")
	}
	if len(got.Errors) == 0 {
		t.Error("expected at least one error")
	}
}

func TestScenarios_Validate_GarbageJSON(t *testing.T) {
	s := server.New()
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/scenarios/validate", "application/json", bytes.NewBufferString("not json {{"))
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
	if body.Error.Code != api.ErrCodeBadRequest {
		t.Errorf("expected code %q, got %q", api.ErrCodeBadRequest, body.Error.Code)
	}
}

func TestScenarios_Validate_WrongContentType(t *testing.T) {
	s := server.New()
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/scenarios/validate", "text/plain", bytes.NewBufferString("hello"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", resp.StatusCode)
	}

	var body api.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != api.ErrCodeUnsupportedMediaType {
		t.Errorf("expected code %q, got %q", api.ErrCodeUnsupportedMediaType, body.Error.Code)
	}
}

func TestScenarios_Validate_WrongMethod(t *testing.T) {
	s := server.New()
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/scenarios/validate")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}
}
