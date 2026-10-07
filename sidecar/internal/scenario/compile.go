package scenario

import (
	"encoding/json"
	"fmt"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
)

// Compile turns a validated Scenario into the JSON args object that
// `kurtosis run` passes to main.star. This is the single source of truth for
// the args shape — Makefile and other callers must not hand-roll it.
//
// Replay scenarios are not compilable here: they flow through
// `confluence replay`, which composes its own kurtosis input from the
// reproducer's source scenario.
func Compile(s *api.Scenario) ([]byte, error) {
	if errs := Validate(s); len(errs) > 0 {
		return nil, fmt.Errorf("scenario invalid: %d error(s); first: %s (%s)", len(errs), errs[0].Message, errs[0].Field)
	}
	if s.Workload.Kind == api.WorkloadReplay {
		return nil, fmt.Errorf("scenario: replay workloads are not compiled directly; use confluence replay")
	}

	out := map[string]any{
		"test_suite":    workloadToTestSuite(s.Workload.Kind, len(s.Chaos.Schedule) > 0),
		"goxrpl_count":  s.Topology.Goxrpl.Count,
		"rippled_count": s.Topology.Rippled.Count,
	}
	// Empty image values intentionally stay absent so Starlark can apply its
	// image defaults. Older scenarios with explicit images retain the legacy
	// JSON shape.
	if s.Topology.Rippled.Image != "" {
		out["rippled_image"] = s.Topology.Rippled.Image
	}
	if s.Topology.Goxrpl.Image != "" {
		out["goxrpl_image"] = s.Topology.Goxrpl.Image
	}
	if s.Topology.Rippled.Entrypoint != nil {
		out["rippled_entrypoint"] = cloneStrings(s.Topology.Rippled.Entrypoint)
	}
	if s.Topology.Rippled.Config != nil {
		out["rippled_config"] = cloneConfig(s.Topology.Rippled.Config)
	}
	if len(s.Topology.Rippled.Nodes) > 0 {
		nodes := make([]map[string]any, len(s.Topology.Rippled.Nodes))
		for i, override := range s.Topology.Rippled.Nodes {
			node := map[string]any{"name": fmt.Sprintf("rippled-%d", i)}
			if override.Image != nil && *override.Image != "" {
				node["image"] = *override.Image
			}
			if override.Entrypoint != nil {
				node["entrypoint"] = cloneStrings(override.Entrypoint)
			}
			if override.Config != nil {
				node["config"] = cloneConfig(override.Config)
			}
			nodes[i] = node
		}
		out["rippled_nodes"] = nodes
	}
	if s.Network != nil {
		network := map[string]any{"network_id": s.EffectiveNetworkID()}
		if s.Network.Amendments != nil {
			network["amendments"] = cloneAmendments(*s.Network.Amendments)
		}
		if s.Network.VetoAmendments != nil {
			network["veto_amendments"] = cloneAmendments(*s.Network.VetoAmendments)
		}
		out["network_config"] = network
	}
	if s.Services != nil {
		if s.Services.Dashboard != nil {
			out["enable_dashboard"] = *s.Services.Dashboard
		}
		if s.Services.Control != nil {
			out["enable_control"] = *s.Services.Control
		}
		if s.Services.SidecarImage != "" {
			out["sidecar_image"] = s.Services.SidecarImage
		}
	}

	// Comma-joined list of enabled oracles (empty = runner defaults to all
	// implemented oracles enabled). Pushed through to the fuzz sidecar via
	// the ORACLES env var; see src/sidecar/fuzz.star.
	oraclesCSV := ""
	for i, o := range s.Oracles {
		if i > 0 {
			oraclesCSV += ","
		}
		oraclesCSV += o
	}

	workArgs := map[string]any{
		"tx_rate":              s.Workload.TxRate,
		"accounts":             s.Workload.Accounts,
		"rotate_every":         s.Workload.RotateEvery,
		"mutation_rate":        s.Workload.MutationRate,
		"submit_workers":       s.Workload.SubmitWorkers,
		"enable_observability": s.Observability.Enabled,
		"alert_webhook_url":    s.Observability.AlertWebhookURL,
		"oracles":              oraclesCSV,
	}

	switch s.Workload.Kind {
	case api.WorkloadSoak:
		out["soak_args"] = workArgs
	case api.WorkloadFuzz:
		// main.star consumes chaos_args.schedule as a JSON-encoded string,
		// not an array — see src/tests/chaos.star and schedule_parse.go.
		scheduleJSON, err := json.Marshal(s.Chaos.Schedule)
		if err != nil {
			return nil, fmt.Errorf("scenario: marshal chaos schedule: %w", err)
		}
		chaosArgs := map[string]any{"schedule": string(scheduleJSON)}
		for k, v := range workArgs {
			chaosArgs[k] = v
		}
		out["chaos_args"] = chaosArgs
	case api.WorkloadNone:
		// no workload args
	}

	return json.Marshal(out)
}

func workloadToTestSuite(kind string, hasSchedule bool) string {
	switch kind {
	case api.WorkloadSoak:
		return "soak"
	case api.WorkloadFuzz:
		if hasSchedule {
			return "chaos"
		}
		// Validate prevents this branch in M1, but keep a coherent fallback.
		return "fuzz"
	case api.WorkloadNone:
		return "none"
	}
	return kind
}

func cloneConfig(config map[string][]string) map[string][]string {
	out := make(map[string][]string, len(config))
	for section, lines := range config {
		out[section] = cloneStrings(lines)
	}
	return out
}

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneAmendments(amendments []api.Amendment) []api.Amendment {
	if amendments == nil {
		return nil
	}
	out := make([]api.Amendment, len(amendments))
	copy(out, amendments)
	return out
}
