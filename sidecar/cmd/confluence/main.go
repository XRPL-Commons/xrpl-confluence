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
	root.AddCommand(newVersionCmd())
	root.AddCommand(newScenarioCmd())
	return root
}
