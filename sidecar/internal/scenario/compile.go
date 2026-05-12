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
		"test_suite":    workloadToTestSuite(s.Workload.Kind),
		"goxrpl_count":  s.Topology.Goxrpl.Count,
		"rippled_count": s.Topology.Rippled.Count,
		"rippled_image": s.Topology.Rippled.Image,
		"goxrpl_image":  s.Topology.Goxrpl.Image,
	}

	workArgs := map[string]any{
		"tx_rate":              s.Workload.TxRate,
		"accounts":             s.Workload.Accounts,
		"rotate_every":         s.Workload.RotateEvery,
		"mutation_rate":        s.Workload.MutationRate,
		"enable_observability": s.Observability.Enabled,
		"alert_webhook_url":    s.Observability.AlertWebhookURL,
	}

	switch s.Workload.Kind {
	case api.WorkloadSoak:
		out["soak_args"] = workArgs
	case api.WorkloadFuzz:
		// Chaos uses fuzz workload + non-empty schedule. Empty schedule = pure fuzz.
		if len(s.Chaos.Schedule) > 0 {
			out["test_suite"] = "chaos"
			chaosArgs := map[string]any{"schedule": s.Chaos.Schedule}
			for k, v := range workArgs {
				chaosArgs[k] = v
			}
			out["chaos_args"] = chaosArgs
		} else {
			out["fuzz_args"] = workArgs
		}
	case api.WorkloadShrink:
		out["shrink_args"] = workArgs
	case api.WorkloadNone:
		// no workload args
	}

	return json.Marshal(out)
}

// workloadToTestSuite maps a scenario workload kind to the existing
// main.star test_suite value. The chaos override is applied in Compile after
// the schedule check.
func workloadToTestSuite(kind string) string {
	switch kind {
	case api.WorkloadSoak:
		return "soak"
	case api.WorkloadFuzz:
		return "fuzz"
	case api.WorkloadShrink:
		return "shrink"
	case api.WorkloadNone:
		return "none"
	}
	return kind
}
