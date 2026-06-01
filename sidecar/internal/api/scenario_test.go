package api

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

const sampleScenarioYAML = `
apiVersion: confluence/v1
kind: Scenario
metadata:
  name: soak-mixed-3x2
  description: 3 rippled + 2 go-xrpl, soak workload
topology:
  rippled:
    count: 3
    image: rippleci/rippled:2.6.2
  goxrpl:
    count: 2
    image: goxrpl:latest
workload:
  kind: soak
  tx_rate: 5
  accounts: 50
  rotate_every: 1000
  mutation_rate: 0.05
chaos:
  schedule: []
observability:
  enabled: false
budget:
  duration: 10m
  stop_on:
    - first_divergence
oracles:
  - state_diff
  - consensus_liveness
  - peer_health
`

func TestScenarioYAMLRoundtrip(t *testing.T) {
	var s Scenario
	if err := yaml.Unmarshal([]byte(sampleScenarioYAML), &s); err != nil {
		t.Fatalf("yaml unmarshal: %v", err)
	}
	if s.APIVersion != "confluence/v1" {
		t.Fatalf("apiVersion: got %q", s.APIVersion)
	}
	if s.Kind != "Scenario" {
		t.Fatalf("kind: got %q", s.Kind)
	}
	if s.Metadata.Name != "soak-mixed-3x2" {
		t.Fatalf("metadata.name: got %q", s.Metadata.Name)
	}
	if s.Topology.Rippled.Count != 3 || s.Topology.Goxrpl.Count != 2 {
		t.Fatalf("topology counts: %+v", s.Topology)
	}
	if s.Workload.Kind != WorkloadSoak {
		t.Fatalf("workload.kind: got %q", s.Workload.Kind)
	}
	if s.Workload.TxRate != 5 || s.Workload.Accounts != 50 {
		t.Fatalf("workload soak fields: %+v", s.Workload)
	}
	if s.Budget.Duration != "10m" {
		t.Fatalf("budget.duration: got %q", s.Budget.Duration)
	}
	if len(s.Budget.StopOn) != 1 || s.Budget.StopOn[0] != StopOnFirstDivergence {
		t.Fatalf("budget.stop_on: %+v", s.Budget.StopOn)
	}
	if len(s.Oracles) != 3 {
		t.Fatalf("oracles: %+v", s.Oracles)
	}

	// JSON marshalling: every field uses snake_case JSON tags.
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	var asMap map[string]any
	if err := json.Unmarshal(b, &asMap); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if _, ok := asMap["api_version"]; !ok {
		t.Fatalf("expected api_version key in JSON, got %s", b)
	}
}
