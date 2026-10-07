package scenario

import (
	"encoding/json"
	"strings"
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
		{"replay empty reproducer id", func(s *api.Scenario) {
			s.Workload.Kind = api.WorkloadReplay
			s.Workload.Reproducer = &api.WorkloadReproducer{}
		}, "scenario_invalid", "workload.reproducer.id"},
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

func TestValidateLocalNetworkSchema(t *testing.T) {
	const amendmentID = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	base := validScenario()
	base.Workload.Kind = api.WorkloadNone
	base.Workload.TxRate = 0
	base.Budget.Duration = ""
	base.Topology.Goxrpl.Count = 0
	base.Topology.Rippled.Count = 2
	base.Topology.Rippled.Nodes = []api.NodeOverride{{Image: stringPtr("rippled:one")}, {Config: map[string][]string{"reporting": {"reporting = true"}}}}
	base.Network = &api.NetworkConfig{
		NetworkID:      intPtr(10001),
		Amendments:     amendmentPtr([]api.Amendment{}),
		VetoAmendments: amendmentPtr([]api.Amendment{{ID: amendmentID, Name: "FeatureFoo"}}),
	}
	dashboard, control := false, false
	base.Services = &api.ServicesConfig{Dashboard: &dashboard, Control: &control}
	if errs := Validate(base); len(errs) != 0 {
		t.Fatalf("valid local network rejected: %+v", errs)
	}

	cases := []struct {
		name      string
		mutate    func(*api.Scenario)
		wantField string
		wantText  string
	}{
		{
			name: "too few nodes",
			mutate: func(s *api.Scenario) {
				s.Topology.Rippled.Count = 1
				s.Topology.Rippled.Nodes = nil
			},
			wantField: "topology",
			wantText:  "at least 2",
		},
		{
			name: "too many nodes",
			mutate: func(s *api.Scenario) {
				s.Topology.Rippled.Count = 11
				s.Topology.Rippled.Nodes = nil
			},
			wantField: "topology",
			wantText:  "at most 10",
		},
		{
			name: "node override length",
			mutate: func(s *api.Scenario) {
				s.Topology.Rippled.Nodes = []api.NodeOverride{{}}
			},
			wantField: "topology.rippled.nodes",
			wantText:  "exactly count",
		},
		{
			name: "custom network mixed",
			mutate: func(s *api.Scenario) {
				s.Workload.Kind = api.WorkloadSoak
				s.Budget.Duration = "1m"
				s.Topology.Goxrpl.Count = 1
			},
			wantField: "network",
			wantText:  "custom network",
		},
		{
			name: "guarded config section",
			mutate: func(s *api.Scenario) {
				s.Topology.Rippled.Config = map[string][]string{"server": {"port = 1"}}
			},
			wantField: "topology.rippled.config.server",
			wantText:  "reserved",
		},
		{
			name: "multiline config",
			mutate: func(s *api.Scenario) {
				s.Topology.Rippled.Config = map[string][]string{"reporting": {"a\nb"}}
			},
			wantField: "topology.rippled.config.reporting[0]",
			wantText:  "single-line",
		},
		{
			name: "overlapping amendment votes",
			mutate: func(s *api.Scenario) {
				s.Network.Amendments = amendmentPtr([]api.Amendment{{ID: amendmentID, Name: "FeatureFoo"}})
			},
			wantField: "network.veto_amendments[0].id",
			wantText:  "also appears",
		},
		{
			name: "runner control required",
			mutate: func(s *api.Scenario) {
				s.Workload.Kind = api.WorkloadSoak
				s.Budget.Duration = "1m"
				s.Network = nil
				s.Topology.Rippled.Config = nil
				control := false
				s.Services = &api.ServicesConfig{Control: &control}
			},
			wantField: "services.control",
			wantText:  "enabled for workload",
		},
		{
			name: "empty per-node image",
			mutate: func(s *api.Scenario) {
				s.Topology.Rippled.Count = 2
				s.Topology.Goxrpl.Count = 0
				s.Topology.Rippled.Nodes = []api.NodeOverride{{Image: stringPtr("")}, {}}
				s.Network = nil
			},
			wantField: "topology.rippled.nodes[0].image",
			wantText:  "non-empty",
		},
		{
			name: "empty group entrypoint",
			mutate: func(s *api.Scenario) {
				s.Topology.Rippled.Entrypoint = []string{}
			},
			wantField: "topology.rippled.entrypoint",
			wantText:  "at least one",
		},
		{
			name: "empty entrypoint command",
			mutate: func(s *api.Scenario) {
				s.Topology.Rippled.Entrypoint = []string{" "}
			},
			wantField: "topology.rippled.entrypoint[0]",
			wantText:  "non-empty",
		},
		{
			name: "whitespace group image",
			mutate: func(s *api.Scenario) {
				s.Topology.Rippled.Image = "  "
			},
			wantField: "topology.rippled.image",
			wantText:  "whitespace",
		},
		{
			name: "whitespace sidecar image",
			mutate: func(s *api.Scenario) {
				s.Services = &api.ServicesConfig{SidecarImage: "  "}
			},
			wantField: "services.sidecar_image",
			wantText:  "whitespace",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := cloneScenario(base)
			tc.mutate(s)
			errs := Validate(s)
			for _, err := range errs {
				if err.Field == tc.wantField && strings.Contains(err.Message, tc.wantText) {
					return
				}
			}
			t.Fatalf("expected field %q containing %q, got %+v", tc.wantField, tc.wantText, errs)
		})
	}
}

func TestValidateAllowsZeroBudgetOnlyForNone(t *testing.T) {
	s := validScenario()
	s.Budget.Duration = "0s"
	if errs := Validate(s); len(errs) == 0 {
		t.Fatal("expected zero duration to be rejected")
	}
	s.Workload.Kind = api.WorkloadNone
	s.Budget.Duration = ""
	s.Topology.Rippled.Count = 2
	s.Topology.Goxrpl.Count = 0
	if errs := Validate(s); len(errs) != 0 {
		t.Fatalf("none workload without budget rejected: %+v", errs)
	}
}

func TestValidateRejectsExplicitEmptyImageFromJSON(t *testing.T) {
	var s api.Scenario
	if err := json.Unmarshal([]byte(`{
"api_version":"confluence/v1",
"kind":"Scenario",
"metadata":{"name":"empty-image"},
"topology":{"rippled":{"count":2,"nodes":[{"image":""},{}]},"goxrpl":{"count":0}},
"workload":{"kind":"none"},
"budget":{}
}`), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	errs := Validate(&s)
	for _, err := range errs {
		if err.Field == "topology.rippled.nodes[0].image" && strings.Contains(err.Message, "non-empty") {
			return
		}
	}
	t.Fatalf("expected explicit empty image rejection, got %+v", errs)
}

func cloneScenario(s *api.Scenario) *api.Scenario {
	clone := *s
	clone.Topology = s.Topology
	clone.Topology.Rippled = s.Topology.Rippled
	clone.Topology.Goxrpl = s.Topology.Goxrpl
	clone.Network = nil
	if s.Network != nil {
		network := *s.Network
		if s.Network.Amendments != nil {
			amendments := make([]api.Amendment, len(*s.Network.Amendments))
			copy(amendments, *s.Network.Amendments)
			network.Amendments = &amendments
		}
		if s.Network.VetoAmendments != nil {
			vetoes := make([]api.Amendment, len(*s.Network.VetoAmendments))
			copy(vetoes, *s.Network.VetoAmendments)
			network.VetoAmendments = &vetoes
		}
		clone.Network = &network
	}
	clone.Services = nil
	if s.Services != nil {
		services := *s.Services
		clone.Services = &services
	}
	clone.Topology.Rippled.Nodes = append([]api.NodeOverride(nil), s.Topology.Rippled.Nodes...)
	clone.Topology.Rippled.Config = cloneConfigForTest(s.Topology.Rippled.Config)
	return &clone
}

func stringPtr(value string) *string { return &value }

func intPtr(value int) *int { return &value }

func amendmentPtr(value []api.Amendment) *[]api.Amendment { return &value }

func cloneConfigForTest(in map[string][]string) map[string][]string {
	if in == nil {
		return nil
	}
	out := make(map[string][]string, len(in))
	for section, lines := range in {
		out[section] = append([]string(nil), lines...)
	}
	return out
}
