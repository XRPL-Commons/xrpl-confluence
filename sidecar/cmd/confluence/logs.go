package main

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/client"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/kurtosis"
	"github.com/spf13/cobra"
)

func newLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Stream logs for a node",
		RunE:  runLogs,
	}
	cmd.Flags().StringP("node", "n", "", "Node name (required)")
	cmd.Flags().Duration("since", 0, "Only return lines from the last DURATION")
	cmd.Flags().String("grep", "", "Filter lines by regex")
	cmd.Flags().BoolP("follow", "f", false, "Follow new log lines after the existing file is exhausted")
	cmd.Flags().Int("limit", 1000, "Max lines to return in non-follow mode")
	return cmd
}

func runLogs(cmd *cobra.Command, _ []string) error {
	node, _ := cmd.Flags().GetString("node")
	if node == "" {
		return fmt.Errorf("--node flag required")
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	controlURL, err := resolveControlURL(ctx, cmd, kurtosis.NewExec())
	if err != nil {
		return err
	}

	since, _ := cmd.Flags().GetDuration("since")
	grep, _ := cmd.Flags().GetString("grep")
	follow, _ := cmd.Flags().GetBool("follow")
	limit, _ := cmd.Flags().GetInt("limit")

	opts := []client.Option{}
	if follow {
		opts = append(opts, client.WithHTTPClient(&http.Client{}))
	}

	c := client.New(controlURL, opts...)

	stream, err := c.Logs(ctx, node, since, grep, follow, limit)
	if err != nil {
		return fmt.Errorf("logs: %w", err)
	}
	defer stream.Close()

	done := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(cmd.OutOrStdout(), stream)
		done <- copyErr
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-done:
		return err
	}
}
