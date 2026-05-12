package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/client"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/discovery"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/kurtosis"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/scenario"
	"github.com/spf13/cobra"
)

func newUpCmd() *cobra.Command {
	return newUpCmdWith(defaultUpDeps())
}

func newUpCmdWith(d *upDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Boot a confluence enclave from a Scenario",
		RunE:  d.run,
	}
	cmd.Flags().StringP("scenario", "f", "", "Path to a Scenario YAML file (required)")
	cmd.Flags().String("enclave", "", "Enclave name (default: derived from scenario name)")
	cmd.Flags().String("package", ".", "Kurtosis package dir (default: current dir)")
	cmd.Flags().Bool("tear-down-first", true, "Tear down any existing enclave with the same name before booting")
	cmd.Flags().Duration("wait-control", 60*time.Second, "How long to wait for control service to become healthy")
	return cmd
}

type upDeps struct {
	cli        kurtosis.CLI
	httpClient *http.Client
}

func defaultUpDeps() *upDeps {
	return &upDeps{
		cli:        kurtosis.NewExec(),
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (d *upDeps) run(cmd *cobra.Command, _ []string) error {
	scenarioPath, _ := cmd.Flags().GetString("scenario")
	if scenarioPath == "" {
		return fmt.Errorf("--scenario flag required")
	}

	enclaveName, _ := cmd.Flags().GetString("enclave")
	packageDir, _ := cmd.Flags().GetString("package")
	tearDownFirst, _ := cmd.Flags().GetBool("tear-down-first")
	waitControl, _ := cmd.Flags().GetDuration("wait-control")

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	cur, err := d.boot(ctx, cmd, scenarioPath, enclaveName, packageDir, tearDownFirst, waitControl)
	if err != nil {
		return err
	}
	return emitUp(cmd, cur)
}

// boot loads, validates, and runs a scenario YAML through kurtosis, waits for
// the control service, writes the discovery file, and returns the Current.
func (d *upDeps) boot(ctx context.Context, cmd *cobra.Command, scenarioPath, enclaveName, packageDir string, tearDownFirst bool, waitControl time.Duration) (*discovery.Current, error) {
	s, err := scenario.Load(scenarioPath)
	if err != nil {
		return nil, outputValidation(cmd, false, []api.Error{{
			Code:    api.ErrCodeScenarioUnreadable,
			Message: err.Error(),
		}})
	}

	if errs := scenario.Validate(s); len(errs) > 0 {
		return nil, outputValidation(cmd, false, errs)
	}

	argsJSON, err := scenario.Compile(s)
	if err != nil {
		return nil, fmt.Errorf("scenario compile: %w", err)
	}

	if enclaveName == "" {
		enclaveName = s.Metadata.Name
	}
	if enclaveName == "" {
		return nil, fmt.Errorf("metadata.name or --enclave required")
	}

	_, err = kurtosis.Run(ctx, d.cli, kurtosis.RunOptions{
		Enclave:       enclaveName,
		PackageDir:    packageDir,
		Args:          argsJSON,
		TearDownFirst: tearDownFirst,
	})
	if err != nil {
		return nil, err
	}

	controlURL, err := d.waitForControl(ctx, enclaveName, waitControl)
	if err != nil {
		return nil, err
	}

	cur := &discovery.Current{
		EnclaveID:  enclaveName,
		ControlURL: controlURL,
		Scenario:   s.Metadata.Name,
		StartedAt:  time.Now().UTC(),
	}
	if err := discovery.Write(cur); err != nil {
		return nil, err
	}
	return cur, nil
}

func (d *upDeps) waitForControl(ctx context.Context, enclave string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for {
		svc, err := kurtosis.InspectService(ctx, d.cli, enclave, "confluence-control")
		if err == nil {
			// Prefer the host-mapped URL (current kurtosis output);
			// fall back to enclave-internal IP for older kurtosis versions.
			controlURL := svc.PortURLs["http"]
			if controlURL == "" && svc.IPAddress != "" {
				controlURL = fmt.Sprintf("http://%s:%d", svc.IPAddress, 8090)
			}
			if controlURL != "" && d.probeHealthz(ctx, controlURL, deadline) {
				return controlURL, nil
			}
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("timed out waiting for confluence-control to become healthy")
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
}

func (d *upDeps) probeHealthz(ctx context.Context, controlURL string, deadline time.Time) bool {
	c := client.New(controlURL, client.WithHTTPClient(d.httpClient))
	for time.Now().Before(deadline) {
		_, err := c.Healthz(ctx)
		if err == nil {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(1 * time.Second):
		}
	}
	return false
}

func emitUp(cmd *cobra.Command, cur *discovery.Current) error {
	asJSON, _ := cmd.Flags().GetBool("json")
	if asJSON {
		payload := struct {
			EnclaveID  string    `json:"enclave_id"`
			ControlURL string    `json:"control_url"`
			Scenario   string    `json:"scenario"`
			StartedAt  time.Time `json:"started_at"`
		}{
			EnclaveID:  cur.EnclaveID,
			ControlURL: cur.ControlURL,
			Scenario:   cur.Scenario,
			StartedAt:  cur.StartedAt,
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Confluence enclave %q ready at %s\n", cur.EnclaveID, cur.ControlURL)
	return nil
}
