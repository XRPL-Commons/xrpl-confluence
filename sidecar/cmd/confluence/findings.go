package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"text/tabwriter"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/client"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/kurtosis"
	"github.com/spf13/cobra"
)

func newFindingsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "findings",
		Short: "List findings from the current enclave",
		RunE:  runFindings,
	}
	cmd.Flags().String("since", "", "Return findings newer than this finding ID")
	cmd.Flags().String("kind", "", "Filter by finding kind")
	cmd.Flags().Int("limit", 100, "Max findings to return (max 1000)")
	return cmd
}

func newFindingCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:   "finding",
		Short: "Inspect individual findings",
	}
	show := &cobra.Command{
		Use:   "show ID",
		Short: "Show one finding",
		Args:  cobra.ExactArgs(1),
		RunE:  runFindingShow,
	}
	parent.AddCommand(show)
	return parent
}

func runFindings(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	controlURL, err := resolveControlURL(ctx, cmd, kurtosis.NewExec())
	if err != nil {
		return err
	}

	since, _ := cmd.Flags().GetString("since")
	kind, _ := cmd.Flags().GetString("kind")
	limit, _ := cmd.Flags().GetInt("limit")

	c := client.New(controlURL)
	findings, err := c.Findings(ctx, since, kind, limit)
	if err != nil {
		return fmt.Errorf("findings: %w", err)
	}

	if jsonMode(cmd) {
		return emitJSON(cmd, findings)
	}

	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tKIND\tOPENED_AT\tSUMMARY")
	for _, f := range findings {
		summary := f.Summary
		if len(summary) > 80 {
			summary = summary[:77] + "..."
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", f.ID, f.Kind, f.OpenedAt.Format("2006-01-02T15:04:05Z"), summary)
	}
	return tw.Flush()
}

func runFindingShow(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	controlURL, err := resolveControlURL(ctx, cmd, kurtosis.NewExec())
	if err != nil {
		return err
	}

	id := args[0]
	c := client.New(controlURL)
	f, err := c.FindingByID(ctx, id)
	if err != nil {
		var apiErr *client.ErrAPI
		if errors.As(err, &apiErr) && apiErr.Status == 404 {
			if jsonMode(cmd) {
				_ = emitJSON(cmd, map[string]any{
					"error": map[string]any{
						"code":    apiErr.Err.Code,
						"message": apiErr.Err.Message,
					},
				})
			}
			return fmt.Errorf("finding not found: %s", id)
		}
		return fmt.Errorf("finding show: %w", err)
	}

	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(b))
	return nil
}
