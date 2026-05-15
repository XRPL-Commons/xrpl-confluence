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
		"rippled_image": s.Topology.Rippled.Image,
		"goxrpl_image":  s.Topology.Goxrpl.Image,
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
