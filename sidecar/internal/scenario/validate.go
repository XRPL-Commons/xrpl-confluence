package scenario

import (
	"fmt"
	"regexp"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
)

// kebabRE matches lowercase kebab-case names (letters, digits, single hyphens).
// We're strict here because the name is reused as a Kurtosis enclave-name
// segment in M2/M3 and exposed in finding records — keeping it conservative
// avoids surprises across the pipeline.
var kebabRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Validate runs all semantic rules over a Scenario and returns a flat list of
// api.Error values with field paths. An empty slice means the scenario is valid.
func Validate(s *api.Scenario) []api.Error {
	var errs []api.Error
	add := func(field, msg, hint string) {
		errs = append(errs, api.Error{
			Code:    api.ErrCodeScenarioInvalid,
			Message: msg,
			Field:   field,
			Hint:    hint,
		})
	}

	if s.APIVersion != api.Version {
		add("apiVersion", fmt.Sprintf("apiVersion must be %q, got %q", api.Version, s.APIVersion), "set apiVersion: confluence/v1")
	}
	if s.Kind != "Scenario" {
		add("kind", fmt.Sprintf("kind must be \"Scenario\", got %q", s.Kind), "set kind: Scenario")
	}

	if s.Metadata.Name == "" {
		add("metadata.name", "metadata.name is required", "")
	} else if !kebabRE.MatchString(s.Metadata.Name) {
		add("metadata.name", fmt.Sprintf("metadata.name must be kebab-case (got %q)", s.Metadata.Name), "use lowercase letters, digits, and single hyphens")
	}

	if s.Topology.Rippled.Count < 0 {
		add("topology.rippled.count", "topology.rippled.count must be >= 0", "")
	}
	if s.Topology.Goxrpl.Count < 0 {
		add("topology.goxrpl.count", "topology.goxrpl.count must be >= 0", "")
	}
	if s.Topology.Rippled.Count+s.Topology.Goxrpl.Count == 0 {
		add("topology", "topology must declare at least one node", "set topology.rippled.count or topology.goxrpl.count > 0")
	}

	switch s.Workload.Kind {
	case api.WorkloadSoak, api.WorkloadNone:
		// fully supported in M1
	case api.WorkloadFuzz:
		// fuzz-only (empty schedule) is not consumed by main.star yet; require chaos schedule.
		if len(s.Chaos.Schedule) == 0 {
			add("workload.kind", "workload.kind=fuzz requires chaos.schedule entries in M1", "either add chaos.schedule entries (becomes a chaos run) or pick a different workload")
		}
	case api.WorkloadShrink:
		// Shrink uses fields not yet modelled (shrink_artifact, shrink_max_step). M2+.
		add("workload.kind", "workload.kind=shrink is not yet supported by the M1 scenario schema", "use confluence shrink directly until M2 adds shrink_args")
	case api.WorkloadReplay:
		if s.Workload.Reproducer == nil || s.Workload.Reproducer.ID == "" {
			add("workload.reproducer.id", "workload.kind=replay requires reproducer.id", "set workload.reproducer.id or change workload.kind")
		}
	case "":
		add("workload.kind", "workload.kind is required", "one of: soak, fuzz, replay, none")
	default:
		add("workload.kind", fmt.Sprintf("unknown workload.kind %q", s.Workload.Kind), "one of: soak, fuzz, replay, none")
	}

	if s.Budget.Duration == "" {
		add("budget.duration", "budget.duration is required", "e.g. \"10m\"")
	} else if _, err := time.ParseDuration(s.Budget.Duration); err != nil {
		add("budget.duration", fmt.Sprintf("budget.duration is not a valid Go duration: %v", err), "use values like \"30s\", \"10m\", \"2h\"")
	}

	allowedStopOn := map[string]bool{
		api.StopOnFirstDivergence: true,
		api.StopOnFirstCrash:      true,
		api.StopOnNone:            true,
	}
	for i, v := range s.Budget.StopOn {
		if !allowedStopOn[v] {
			add(fmt.Sprintf("budget.stop_on[%d]", i), fmt.Sprintf("unknown stop_on value %q", v), "one of: first_divergence, first_crash, none")
		}
	}

	allowedOracle := map[string]bool{
		api.OracleStateDiff:         true,
		api.OracleConsensusLiveness: true,
		api.OraclePeerHealth:        true,
	}
	for i, v := range s.Oracles {
		if !allowedOracle[v] {
			add(fmt.Sprintf("oracles[%d]", i), fmt.Sprintf("unknown oracle %q", v), "one of: state_diff, consensus_liveness, peer_health")
		}
	}

	return errs
}
