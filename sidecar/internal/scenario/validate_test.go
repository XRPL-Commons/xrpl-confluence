package scenario

import (
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
)

func validScenario() *api.Scenario {
	return &api.Scenario{
		APIVersion: "confluence/v1",
		Kind:       "Scenario",
		Metadata:   api.ScenarioMetadata{Name: "soak-mixed-3x2"},
		Topology: api.Topology{
			Rippled: api.NodeGroup{Count: 3, Image: "rippleci/rippled:2.6.2"},
			Goxrpl:  api.NodeGroup{Count: 2, Image: "goxrpl:latest"},
		},
		Workload: api.Workload{Kind: api.WorkloadSoak},
		Budget:   api.Budget{Duration: "10m", StopOn: []string{api.StopOnFirstDivergence}},
		Oracles:  []string{api.OracleStateDiff},
	}
}

func TestValidateAcceptsValidScenario(t *testing.T) {
	errs := Validate(validScenario())
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %+v", errs)
	}
}

func TestValidateRules(t *testing.T) {
	cases := []struct {
		name      string
		mutate    func(*api.Scenario)
		wantCode  string
		wantField string
	}{
		{"bad apiVersion", func(s *api.Scenario) { s.APIVersion = "confluence/v0" }, "scenario_invalid", "apiVersion"},
		{"bad kind", func(s *api.Scenario) { s.Kind = "NotScenario" }, "scenario_invalid", "kind"},
		{"missing name", func(s *api.Scenario) { s.Metadata.Name = "" }, "scenario_invalid", "metadata.name"},
		{"non-kebab name", func(s *api.Scenario) { s.Metadata.Name = "Soak Mixed" }, "scenario_invalid", "metadata.name"},
		{"no nodes", func(s *api.Scenario) { s.Topology.Rippled.Count = 0; s.Topology.Goxrpl.Count = 0 }, "scenario_invalid", "topology"},
		{"negative count", func(s *api.Scenario) { s.Topology.Rippled.Count = -1 }, "scenario_invalid", "topology.rippled.count"},
		{"bad workload kind", func(s *api.Scenario) { s.Workload.Kind = "explode" }, "scenario_invalid", "workload.kind"},
		{"fuzz without schedule", func(s *api.Scenario) { s.Workload.Kind = api.WorkloadFuzz }, "scenario_invalid", "workload.kind"},
		{"shrink not supported", func(s *api.Scenario) { s.Workload.Kind = api.WorkloadShrink }, "scenario_invalid", "workload.kind"},
		{"replay missing reproducer", func(s *api.Scenario) { s.Workload.Kind = api.WorkloadReplay }, "scenario_invalid", "workload.reproducer.id"},
		{"replay empty reproducer id", func(s *api.Scenario) { s.Workload.Kind = api.WorkloadReplay; s.Workload.Reproducer = &api.WorkloadReproducer{} }, "scenario_invalid", "workload.reproducer.id"},
		{"missing budget duration", func(s *api.Scenario) { s.Budget.Duration = "" }, "scenario_invalid", "budget.duration"},
		{"bad budget duration", func(s *api.Scenario) { s.Budget.Duration = "ten minutes" }, "scenario_invalid", "budget.duration"},
		{"bad stop_on", func(s *api.Scenario) { s.Budget.StopOn = []string{"yolo"} }, "scenario_invalid", "budget.stop_on[0]"},
		{"bad oracle", func(s *api.Scenario) { s.Oracles = []string{"nope"} }, "scenario_invalid", "oracles[0]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := validScenario()
			tc.mutate(s)
			errs := Validate(s)
			if len(errs) == 0 {
				t.Fatalf("expected error for %s, got none", tc.name)
			}
			var matched bool
			for _, e := range errs {
				if e.Code == tc.wantCode && e.Field == tc.wantField {
					matched = true
					break
				}
			}
			if !matched {
				t.Fatalf("expected code=%q field=%q, got %+v", tc.wantCode, tc.wantField, errs)
			}
		})
	}
}
