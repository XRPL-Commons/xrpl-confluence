package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"regexp"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/kurtosis"
	"github.com/spf13/cobra"
)

func newLogsCmd() *cobra.Command {
	return newLogsCmdWith(&logsDeps{cli: kurtosis.NewExec()})
}

func newLogsCmdWith(d *logsDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Stream logs for a node",
		RunE:  d.run,
	}
	cmd.Flags().StringP("node", "n", "", "Node name (required)")
	cmd.Flags().Duration("since", 0, "Only return lines from the last DURATION")
	cmd.Flags().String("grep", "", "Filter lines by regex")
	cmd.Flags().BoolP("follow", "f", false, "Follow new log lines after the existing file is exhausted")
	cmd.Flags().Int("limit", 1000, "Max lines to return in non-follow mode")
	return cmd
}

type logsDeps struct {
	cli kurtosis.CLI
}

func (d *logsDeps) run(cmd *cobra.Command, _ []string) error {
	node, _ := cmd.Flags().GetString("node")
	if node == "" {
		return fmt.Errorf("--node flag required")
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	enclave, err := resolveEnclave(cmd, ctx, d.cli)
	if err != nil {
		return fmt.Errorf("logs: %w", err)
	}

	follow, _ := cmd.Flags().GetBool("follow")
	grep, _ := cmd.Flags().GetString("grep")

	w := cmd.OutOrStdout()
	if grep != "" {
		re, err := regexp.Compile(grep)
		if err != nil {
			return fmt.Errorf("--grep: invalid regex: %w", err)
		}
		pr, pw := io.Pipe()
		go func() {
			err := kurtosis.ServiceLogs(ctx, d.cli, enclave, node, follow, pw)
			pw.CloseWithError(err)
		}()
		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			line := scanner.Text()
			if re.MatchString(line) {
				fmt.Fprintln(w, line)
			}
		}
		return scanner.Err()
	}

	return kurtosis.ServiceLogs(ctx, d.cli, enclave, node, follow, w)
}
