package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/client"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/scenario"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/server"
	"github.com/spf13/cobra"
)

func newRunCmd() *cobra.Command {
	return newRunCmdWith(defaultUpDeps(), defaultDownDeps())
}

func newRunCmdWith(up *upDeps, down *downDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run SCENARIO",
		Short: "Boot an enclave, run a scenario, wait for completion or stop_on, optionally tear down",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRun(cmd, args, up, down)
		},
	}
	cmd.Flags().BoolP("wait", "w", true, "Wait for budget elapsed or stop_on trigger")
	cmd.Flags().Duration("timeout", 0, "Hard CLI-side timeout (default: 2x scenario budget)")
	cmd.Flags().Bool("down", true, "Tear down the enclave when the run finishes")
	cmd.Flags().Bool("tear-down-first", true, "Tear down any existing enclave with the same name before booting")
	cmd.Flags().Duration("wait-control", 60*time.Second, "How long to wait for control service to become healthy")
	cmd.Flags().String("package", ".", "Kurtosis package dir")
	return cmd
}

func runRun(cmd *cobra.Command, args []string, up *upDeps, down *downDeps) error {
	scenarioPath := args[0]

	// Load and validate scenario upfront so we can compute the timeout.
	s, err := scenario.Load(scenarioPath)
	if err != nil {
		return outputValidation(cmd, false, []api.Error{{
			Code:    api.ErrCodeScenarioUnreadable,
			Message: err.Error(),
		}})
	}
	if errs := scenario.Validate(s); len(errs) > 0 {
		return outputValidation(cmd, false, errs)
	}

	waitFlag, _ := cmd.Flags().GetBool("wait")
	doDown, _ := cmd.Flags().GetBool("down")
	tearDownFirst, _ := cmd.Flags().GetBool("tear-down-first")
	waitControl, _ := cmd.Flags().GetDuration("wait-control")
	packageDir, _ := cmd.Flags().GetString("package")
	timeoutFlag, _ := cmd.Flags().GetDuration("timeout")

	hardTimeout := timeoutFlag
	if hardTimeout == 0 {
		budgetDur, _ := time.ParseDuration(s.Budget.Duration)
		hardTimeout = 2 * budgetDur
		if hardTimeout == 0 {
			hardTimeout = 10 * time.Minute
		}
	}

	rootCtx := cmd.Context()
	if rootCtx == nil {
		rootCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(rootCtx, hardTimeout)
	defer cancel()

	// Boot the enclave.
	cur, err := up.boot(ctx, cmd, scenarioPath, "", packageDir, tearDownFirst, waitControl)
	if err != nil {
		return err
	}

	// Reuse upDeps' HTTP client so that tests can inject a redirect transport.
	apiHTTPClient := up.httpClient
	if apiHTTPClient == nil {
		apiHTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	c := client.New(cur.ControlURL, client.WithHTTPClient(apiHTTPClient))

	// Start the run.
	run, err := c.StartRun(ctx, s)
	if err != nil {
		return fmt.Errorf("start run: %w", err)
	}

	startTime := time.Now()

	// Wait loop.
	if waitFlag {
		run, err = pollRun(ctx, cmd, c, run)
		if err != nil {
			return err
		}
	}

	durationMS := time.Since(startTime).Milliseconds()

	// Optional teardown.
	if doDown {
		if _, err := down.tearDown(ctx, cur.EnclaveID); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: teardown failed: %v\n", err)
		}
	}

	// Output.
	exitCode := exitCodeForRun(run)
	if jsonMode(cmd) {
		payload := map[string]any{
			"run_id":          run.ID,
			"status":          run.Status,
			"scenario":        run.Scenario,
			"started_at":      run.StartedAt,
			"ended_at":        run.EndedAt,
			"finding_ids":     run.FindingIDs,
			"trigger_finding": run.TriggerFinding,
			"reproducer_ids":  run.ReproducerIDs,
			"enclave_id":      cur.EnclaveID,
			"control_url":     cur.ControlURL,
			"duration_ms":     durationMS,
		}
		if err := emitJSON(cmd, payload); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Run %s %s\n", run.ID, run.Status)
		fmt.Fprintf(cmd.OutOrStdout(), "Enclave: %s\n", cur.EnclaveID)
		fmt.Fprintf(cmd.OutOrStdout(), "Duration: %d ms\n", durationMS)
		fmt.Fprintf(cmd.OutOrStdout(), "Findings: %d\n", len(run.FindingIDs))
		if run.TriggerFinding != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Trigger finding: %s\n", run.TriggerFinding)
		}
		if len(run.ReproducerIDs) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "Reproducer: %s\n", run.ReproducerIDs[0])
		}
	}

	if exitCode != 0 {
		return exitCodeError(exitCode)
	}
	return nil
}

// pollRun polls GetRun every second until the run is no longer running or ctx is done.
func pollRun(ctx context.Context, cmd *cobra.Command, c *client.Client, run server.Run) (server.Run, error) {
	start := time.Now()
	for run.Status == server.RunStatusRunning {
		select {
		case <-ctx.Done():
			return run, ctx.Err()
		case <-time.After(1 * time.Second):
		}

		updated, err := c.GetRun(ctx, run.ID)
		if err != nil {
			return run, fmt.Errorf("poll run: %w", err)
		}
		run = updated

		if !jsonMode(cmd) {
			elapsed := int(time.Since(start).Seconds())
			fmt.Fprintf(cmd.ErrOrStderr(), "[%ds] status=%s findings=%d\n", elapsed, run.Status, len(run.FindingIDs))
		}
	}
	return run, nil
}

// exitCodeForRun maps a run status to a CLI exit code.
//   - 0: completed_budget with no findings
//   - 3: completed_stop_on OR any findings present
//   - 1: other/error
func exitCodeForRun(run server.Run) int {
	if run.Status == server.RunStatusCompletedBudget && len(run.FindingIDs) == 0 {
		return 0
	}
	if run.Status == server.RunStatusCompletedStopOn || len(run.FindingIDs) > 0 {
		return 3
	}
	return 1
}

// exitCodeError is a sentinel error that carries a non-zero exit code.
// cobra's SilenceErrors suppresses the default error output.
type exitCodeError int

func (e exitCodeError) Error() string { return fmt.Sprintf("exit %d", int(e)) }
