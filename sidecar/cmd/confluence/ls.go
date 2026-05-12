package main

import (
	"context"
	"fmt"
	"net/http"
	"text/tabwriter"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/client"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/kurtosis"
	"github.com/spf13/cobra"
)

func newLsCmd() *cobra.Command {
	return newLsCmdWith(&lsDeps{cli: kurtosis.NewExec()})
}

func newLsCmdWith(d *lsDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List confluence enclaves",
		RunE:  d.run,
	}
}

type lsDeps struct {
	cli        kurtosis.CLI
	httpClient *http.Client
}

type enclaveRow struct {
	EnclaveID  string `json:"enclave_id"`
	Scenario   string `json:"scenario"`
	Status     string `json:"status"`
	ControlURL string `json:"control_url"`
	StartedAt  string `json:"started_at"`
}

func (d *lsDeps) run(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	names, err := kurtosis.ListEnclaves(ctx, d.cli)
	if err != nil {
		return fmt.Errorf("ls: %w", err)
	}

	rows := make([]enclaveRow, 0, len(names))
	for _, name := range names {
		row := d.probe(ctx, name)
		rows = append(rows, row)
	}

	if jsonMode(cmd) {
		return emitJSON(cmd, rows)
	}

	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ENCLAVE\tSCENARIO\tSTATUS\tCONTROL_URL\tSTARTED_AT")
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.EnclaveID, r.Scenario, r.Status, r.ControlURL, r.StartedAt)
	}
	return tw.Flush()
}

func (d *lsDeps) probe(ctx context.Context, name string) enclaveRow {
	row := enclaveRow{EnclaveID: name, Status: "unhealthy"}

	svc, err := kurtosis.InspectService(ctx, d.cli, name, "confluence-control")
	if err != nil || svc.IPAddress == "" {
		row.Status = fmt.Sprintf("unhealthy: %v", err)
		return row
	}

	controlURL := fmt.Sprintf("http://%s:8090", svc.IPAddress)
	row.ControlURL = controlURL

	opts := []client.Option{}
	if d.httpClient != nil {
		opts = append(opts, client.WithHTTPClient(d.httpClient))
	}
	c := client.New(controlURL, opts...)
	h, err := c.Healthz(ctx)
	if err != nil {
		row.Status = fmt.Sprintf("unhealthy: %v", err)
		return row
	}

	row.Status = "ok"
	row.Scenario = h.Scenario
	if h.UptimeS > 0 {
		startedAt := time.Now().Add(-time.Duration(h.UptimeS) * time.Second)
		row.StartedAt = startedAt.UTC().Format(time.RFC3339)
	}
	return row
}
