package main

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

func newReplayCmd() *cobra.Command {
	return newReplayCmdWith(defaultUpDeps(), defaultDownDeps())
}

func newReplayCmdWith(up *upDeps, down *downDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "replay REPRODUCER_ID",
		Short: "Boot an enclave from a reproducer YAML",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReplay(cmd, args, up, down)
		},
	}
	cmd.Flags().BoolP("wait", "w", true, "Wait for budget or stop_on")
	cmd.Flags().Duration("timeout", 0, "Hard CLI-side timeout")
	cmd.Flags().Bool("down", true, "Tear down on finish")
	cmd.Flags().Bool("tear-down-first", true, "Tear down any existing enclave before booting")
	cmd.Flags().Duration("wait-control", 60*time.Second, "How long to wait for control service to become healthy")
	cmd.Flags().String("package", ".", "Kurtosis package dir")
	cmd.Flags().String("dest", ".confluence", "Reproducer root (default .confluence)")
	return cmd
}

func runReplay(cmd *cobra.Command, args []string, up *upDeps, down *downDeps) error {
	reproducerID := args[0]
	dest, _ := cmd.Flags().GetString("dest")

	scenarioPath := filepath.Join(dest, "reproducers", reproducerID+".yaml")
	if !fileExists(scenarioPath) {
		return fmt.Errorf("reproducer not found locally at %s; run 'confluence pull' first", scenarioPath)
	}

	return runRun(cmd, []string{scenarioPath}, up, down)
}
