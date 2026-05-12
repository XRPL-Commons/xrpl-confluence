package main

import (
	"encoding/json"
	"fmt"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
	"github.com/spf13/cobra"
)

// Version is the CLI's own version. Wired by ldflags in releases; defaults
// to "dev" for local builds.
var Version = "dev"

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print CLI and API versions",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := struct {
				Version    string `json:"version"`
				APIVersion string `json:"api_version"`
			}{Version, api.Version}

			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "confluence %s (api %s)\n", out.Version, out.APIVersion)
			return nil
		},
	}
}
