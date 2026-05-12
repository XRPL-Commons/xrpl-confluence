package main

import (
	"context"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/client"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/kurtosis"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/server"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show network status of the current enclave",
		RunE:  runStatus,
	}
	cmd.Flags().BoolP("watch", "w", false, "Re-render every 2s until Ctrl-C")
	return cmd
}

type statusSnapshot struct {
	Healthz       server.HealthzResponse   `json:"healthz"`
	Nodes         server.NodesResponse     `json:"nodes"`
	LatestFinding *api.Finding             `json:"latest_finding"`
	StateDiff     server.StateDiffResponse `json:"state_diff"`
}

func runStatus(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	controlURL, err := resolveControlURL(ctx, cmd, kurtosis.NewExec())
	if err != nil {
		return err
	}

	c := client.New(controlURL)
	watch, _ := cmd.Flags().GetBool("watch")

	render := func() error {
		snap, err := fetchStatus(ctx, c)
		if err != nil {
			return err
		}
		if jsonMode(cmd) {
			return emitJSON(cmd, snap)
		}
		printStatus(cmd, snap)
		return nil
	}

	if !watch {
		return render()
	}

	for {
		fmt.Fprint(cmd.OutOrStdout(), "\x1b[2J\x1b[H")
		if err := render(); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(2 * time.Second):
		}
	}
}

func fetchStatus(ctx context.Context, c *client.Client) (statusSnapshot, error) {
	h, err := c.Healthz(ctx)
	if err != nil {
		return statusSnapshot{}, fmt.Errorf("healthz: %w", err)
	}
	nodes, err := c.Nodes(ctx)
	if err != nil {
		return statusSnapshot{}, fmt.Errorf("nodes: %w", err)
	}
	findings, err := c.Findings(ctx, "", "", 1)
	if err != nil {
		return statusSnapshot{}, fmt.Errorf("findings: %w", err)
	}
	diff, err := c.StateDiff(ctx, 0)
	if err != nil {
		return statusSnapshot{}, fmt.Errorf("state/diff: %w", err)
	}

	snap := statusSnapshot{
		Healthz:   h,
		Nodes:     nodes,
		StateDiff: diff,
	}
	if len(findings) > 0 {
		snap.LatestFinding = &findings[0]
	}
	return snap, nil
}

func printStatus(cmd *cobra.Command, snap statusSnapshot) {
	w := cmd.OutOrStdout()

	h := snap.Healthz
	budgetStr := "n/a"
	if h.BudgetRemainingS != nil {
		budgetStr = fmt.Sprintf("%ds", *h.BudgetRemainingS)
	}
	fmt.Fprintf(w, "Health:  scenario=%s  uptime=%ds  budget=%s\n\n", h.Scenario, h.UptimeS, budgetStr)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tTYPE\tSTATUS\tSEQ\tHASH\tPEERS")
	for _, n := range snap.Nodes.Nodes {
		seq := ""
		hash := ""
		if n.ValidatedLedger != nil {
			seq = fmt.Sprintf("%d", n.ValidatedLedger.Seq)
			if len(n.ValidatedLedger.Hash) > 8 {
				hash = n.ValidatedLedger.Hash[:8]
			} else {
				hash = n.ValidatedLedger.Hash
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\n", n.Name, n.Type, n.Status, seq, hash, n.Peers)
	}
	_ = tw.Flush()

	d := snap.StateDiff
	fmt.Fprintf(w, "\nState diff:  ledger=%d  diverged=%v\n", d.Ledger, d.Diverged)

	fmt.Fprintf(w, "\nLatest finding: ")
	if snap.LatestFinding == nil {
		fmt.Fprintln(w, "no findings yet")
	} else {
		f := snap.LatestFinding
		fmt.Fprintf(w, "%s  %s  %s\n", f.ID, f.Kind, f.Summary)
	}
}

