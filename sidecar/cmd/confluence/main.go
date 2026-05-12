// Package main is the confluence CLI entrypoint.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "confluence",
		Short:         "Drive xrpl-confluence enclaves",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().Bool("json", false, "Emit machine-readable JSON on stdout")
	root.PersistentFlags().String("enclave", "", "Enclave name (used to resolve control URL via kurtosis)")
	root.PersistentFlags().String("control-url", "", "Override control service URL (e.g. http://1.2.3.4:8090)")
	root.AddCommand(newVersionCmd())
	root.AddCommand(newScenarioCmd())
	root.AddCommand(newUpCmd())
	root.AddCommand(newDownCmd())
	root.AddCommand(newLsCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newFindingsCmd())
	root.AddCommand(newFindingCmd())
	root.AddCommand(newLogsCmd())
	root.AddCommand(newPullCmd())
	root.AddCommand(newEventsCmd())
	return root
}
