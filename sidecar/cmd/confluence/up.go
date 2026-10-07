package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/client"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/discovery"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/kurtosis"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/scenario"
	"github.com/spf13/cobra"
)

// execDockerBuilder is the production dockerBuilder; it shells out to the
// `docker` binary on PATH. Errors propagate the combined stdout+stderr so
// build failures are visible in the CLI output.
type execDockerBuilder struct{}

func (execDockerBuilder) Build(ctx context.Context, dir, tag string, stderr io.Writer) error {
	c := exec.CommandContext(ctx, "docker", "build", "-t", tag, dir)
	c.Stdout = stderr
	c.Stderr = stderr
	return c.Run()
}

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
	cmd.Flags().String("package", ".", "Kurtosis package dir (default: current dir)")
	cmd.Flags().Bool("resume", false, "Restart a local network using its existing ledger data")
	cmd.Flags().Bool("reset", false, "Delete the existing local network and its ledger data before booting")
	cmd.MarkFlagsMutuallyExclusive("resume", "reset")
	cmd.Flags().Duration("wait-network", 180*time.Second, "Timeout for local-network validated ledger agreement and progress")
	cmd.Flags().Bool("tear-down-first", true, "Tear down any existing enclave with the same name before booting")
	cmd.Flags().Duration("wait-control", 60*time.Second, "How long to wait for control service to become healthy")
	cmd.Flags().Duration("boot-hang-threshold", 90*time.Second, "Kill the kurtosis CLI if it stays silent this long (watchdog for the 1-in-3 0% CPU hangs); 0 disables")
	cmd.Flags().String("rebuild-goxrpl", "", "docker build this dir and tag it with the scenario's goxrpl image before booting")
	cmd.Flags().String("rebuild-rippled", "", "docker build this dir and tag it with the scenario's rippled image before booting")
	cmd.Flags().Bool("with-dashboard", false, "Force the grafana observability sidecar on regardless of the scenario YAML")
	return cmd
}

type upDeps struct {
	cli        kurtosis.CLI
	httpClient *http.Client
	docker     dockerBuilder
}

// dockerBuilder is the tiny surface area the boot path needs from docker.
// Defined here rather than in a sub-package so tests can stub it without
// pulling docker as a dependency. The real implementation shells out via
// the system docker binary.
type dockerBuilder interface {
	Build(ctx context.Context, dir, tag string, stderr io.Writer) error
}

func defaultUpDeps() *upDeps {
	return &upDeps{
		cli:        kurtosis.NewExec(),
		httpClient: &http.Client{Timeout: 5 * time.Second},
		docker:     execDockerBuilder{},
	}
}

func (d *upDeps) run(cmd *cobra.Command, _ []string) error {
	scenarioPath, _ := cmd.Flags().GetString("scenario")
	if scenarioPath == "" {
		return fmt.Errorf("--scenario flag required")
	}

	enclaveName, _ := cmd.Root().PersistentFlags().GetString("enclave")
	resume, _ := cmd.Flags().GetBool("resume")
	reset, _ := cmd.Flags().GetBool("reset")
	waitNetwork, _ := cmd.Flags().GetDuration("wait-network")
	if waitNetwork < time.Second {
		return fmt.Errorf("--wait-network must be at least 1s")
	}
	packageDir, _ := cmd.Flags().GetString("package")
	tearDownFirst, _ := cmd.Flags().GetBool("tear-down-first")
	waitControl, _ := cmd.Flags().GetDuration("wait-control")
	bootHang, _ := cmd.Flags().GetDuration("boot-hang-threshold")
	rebuildGoXRPL, _ := cmd.Flags().GetString("rebuild-goxrpl")
	rebuildRippled, _ := cmd.Flags().GetString("rebuild-rippled")
	withDashboard, _ := cmd.Flags().GetBool("with-dashboard")

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	cur, err := d.boot(ctx, cmd, bootOptions{
		ScenarioPath:      scenarioPath,
		EnclaveName:       enclaveName,
		PackageDir:        packageDir,
		TearDownFirst:     tearDownFirst,
		WaitControl:       waitControl,
		BootHangThreshold: bootHang,
		RebuildGoXRPL:     rebuildGoXRPL,
		RebuildRippled:    rebuildRippled,
		WithDashboard:     withDashboard,
		Resume:            resume,
		Reset:             reset,
		WaitNetwork:       waitNetwork,
	})
	if err != nil {
		return err
	}
	return emitUp(cmd, cur)
}

// bootOptions bundles the inputs to boot. Grouping them prevents the call
// site from growing yet another positional argument every time we wire a new
// flag (rebuild images, with-dashboard, ...).
type bootOptions struct {
	ScenarioPath      string
	EnclaveName       string
	PackageDir        string
	TearDownFirst     bool
	WaitControl       time.Duration
	BootHangThreshold time.Duration
	// RebuildGoXRPL / RebuildRippled: when non-empty, docker build the named
	// directory and tag it with the scenario's topology image before booting.
	RebuildGoXRPL  string
	RebuildRippled string
	// WithDashboard forces Observability.Enabled = true regardless of what
	// the scenario YAML says, so long-session callers don't have to edit YAML
	// just to flip the grafana sidecar on.
	WithDashboard bool
	// BudgetOverride, when non-zero, replaces the scenario's budget.duration
	// after load (so the compile pass and the control-service budget both see
	// the override). Used by `confluence run --budget 8h`.
	BudgetOverride time.Duration
	Resume         bool
	Reset          bool
	WaitNetwork    time.Duration
}

// boot loads, validates, and runs a scenario YAML through kurtosis, waits for
// the control service, writes the discovery file, and returns the Current.
func (d *upDeps) boot(ctx context.Context, cmd *cobra.Command, o bootOptions) (*discovery.Current, error) {
	scenarioPath := o.ScenarioPath
	enclaveName := o.EnclaveName
	packageDir := o.PackageDir
	tearDownFirst := o.TearDownFirst
	waitControl := o.WaitControl
	s, err := scenario.Load(scenarioPath)
	if err != nil {
		return nil, outputValidation(cmd, false, []api.Error{{
			Code:    api.ErrCodeScenarioUnreadable,
			Message: err.Error(),
		}})
	}

	if o.WithDashboard {
		s.Observability.Enabled = true
	}
	if o.BudgetOverride > 0 {
		s.Budget.Duration = o.BudgetOverride.String()
	}

	if errs := scenario.Validate(s); len(errs) > 0 {
		return nil, outputValidation(cmd, false, errs)
	}

	if enclaveName == "" {
		enclaveName = s.Metadata.Name
	}
	if enclaveName == "" {
		return nil, fmt.Errorf("metadata.name or --enclave required")
	}
	networkOnly := s.Workload.Kind == api.WorkloadNone
	if (o.Resume || o.Reset) && !networkOnly {
		return nil, fmt.Errorf("--resume and --reset require workload.kind: none")
	}
	if networkOnly {
		if err := d.checkNetworkLifecycle(ctx, s, enclaveName, o); err != nil {
			return nil, err
		}
		tearDownFirst = false
	}

	// Image rebuilds run BEFORE compile so the topology image fields are
	// already in their final state. We tag with the scenario's topology image
	// (or its default) so the image kurtosis pulls is byte-for-byte the one
	// we just built — no stale-cache surprises.
	if o.RebuildGoXRPL != "" {
		tag := s.Topology.Goxrpl.Image
		if tag == "" {
			tag = "goxrpl:latest"
		}
		if err := d.docker.Build(ctx, o.RebuildGoXRPL, tag, cmd.ErrOrStderr()); err != nil {
			return nil, fmt.Errorf("rebuild goxrpl: %w", err)
		}
		s.Topology.Goxrpl.Image = tag
	}
	if o.RebuildRippled != "" {
		tag := s.Topology.Rippled.Image
		if tag == "" {
			tag = "rippled:latest"
		}
		if err := d.docker.Build(ctx, o.RebuildRippled, tag, cmd.ErrOrStderr()); err != nil {
			return nil, fmt.Errorf("rebuild rippled: %w", err)
		}
		s.Topology.Rippled.Image = tag
	}

	argsJSON, err := scenario.Compile(s)
	if err != nil {
		return nil, fmt.Errorf("scenario compile: %w", err)
	}

	bootHang := o.BootHangThreshold
	maxAttempts := 3
	if networkOnly {
		maxAttempts = 1
		if bootHang > 0 && bootHang < o.WaitNetwork+time.Minute {
			bootHang = o.WaitNetwork + time.Minute
		}
		argsJSON, err = networkRunArgs(argsJSON, o)
		if err != nil {
			return nil, err
		}
		if o.Reset {
			if err := kurtosis.RemoveEnclave(ctx, d.cli, enclaveName); err != nil {
				return nil, err
			}
			if current, err := discovery.Read(); err == nil && current.EnclaveID == enclaveName {
				if err := discovery.Remove(); err != nil {
					return nil, err
				}
			}
		}
		if err := discovery.WriteNetwork(enclaveName, argsJSON); err != nil {
			return nil, err
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "Starting network %q; waiting for advancing, matching validated ledgers...\n", enclaveName)
	}

	_, err = kurtosis.Run(ctx, d.cli, kurtosis.RunOptions{
		Enclave:           enclaveName,
		PackageDir:        packageDir,
		Args:              argsJSON,
		TearDownFirst:     tearDownFirst,
		MaxAttempts:       maxAttempts,
		BootHangThreshold: bootHang,
		OnRetry: func(attempt int, prev error) {
			fmt.Fprintf(cmd.ErrOrStderr(),
				"kurtosis run transient failure (attempt %d/3): %v\nretrying with a fresh enclave...\n",
				attempt-1, prev)
		},
		OnBootHang: func(silenceFor time.Duration) {
			fmt.Fprintf(cmd.ErrOrStderr(),
				"kurtosis boot watchdog tripped after %.0fs of silence; stopping the boot attempt...\n",
				silenceFor.Seconds())
		},
	})
	if err != nil {
		return nil, err
	}

	controlURL := ""
	if controlEnabled(argsJSON) {
		controlURL, err = d.waitForControl(ctx, enclaveName, waitControl)
		if err != nil {
			return nil, err
		}
	}

	cur := &discovery.Current{
		EnclaveID:  enclaveName,
		ControlURL: controlURL,
		Scenario:   s.Metadata.Name,
		StartedAt:  time.Now().UTC(),
	}
	if networkOnly {
		if err := d.discoverNetwork(ctx, cur, s.Topology.Rippled.Count, s.Topology.Goxrpl.Count, argsJSON); err != nil {
			return nil, err
		}
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
	if jsonMode(cmd) {
		return emitJSON(cmd, cur)
	}
	if cur.ControlURL != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Confluence enclave %q ready at %s\n", cur.EnclaveID, cur.ControlURL)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Confluence enclave %q ready\n", cur.EnclaveID)
	}
	return printEndpoints(cmd.OutOrStdout(), cur)
}
