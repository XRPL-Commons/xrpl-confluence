package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/discovery"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/kurtosis"
	"github.com/spf13/cobra"
)

// resolveControlURL returns the control URL to use, in priority order:
//  1. --control-url flag (if set)
//  2. --enclave flag → kurtosis InspectService → http://<ip>:8090
//  3. discovery.Read() → its ControlURL field
func resolveControlURL(ctx context.Context, cmd *cobra.Command, cli kurtosis.CLI) (string, error) {
	if u, _ := cmd.Root().PersistentFlags().GetString("control-url"); u != "" {
		return u, nil
	}

	if enclave, _ := cmd.Root().PersistentFlags().GetString("enclave"); enclave != "" {
		svc, err := kurtosis.InspectService(ctx, cli, enclave, "confluence-control")
		if err != nil {
			return "", fmt.Errorf("kurtosis inspect for --enclave %q: %w", enclave, err)
		}
		// Prefer the host-published URL (e.g. http://127.0.0.1:54944): the
		// enclave-internal bridge IP returned by `kurtosis service inspect` is
		// unreachable from the host on Docker Desktop for Mac, and is reported
		// empty even when the service is RUNNING.
		if hostURL, ok := svc.PortURLs["http"]; ok && hostURL != "" {
			return hostURL, nil
		}
		if svc.IPAddress != "" {
			return fmt.Sprintf("http://%s:8090", svc.IPAddress), nil
		}
		return "", fmt.Errorf("confluence-control service in enclave %q has neither a host-published http port nor an IP address", enclave)
	}

	cur, err := discovery.Read()
	if err != nil {
		return "", fmt.Errorf("no --control-url, no --enclave, and no discovery file: %w", err)
	}
	if cur.ControlURL == "" {
		return "", fmt.Errorf("discovery file has no control_url; re-run with --control-url or --enclave")
	}
	return cur.ControlURL, nil
}

// jsonMode returns true if --json is set anywhere up the command tree.
func jsonMode(cmd *cobra.Command) bool {
	v, _ := cmd.Root().PersistentFlags().GetBool("json")
	return v
}

// emitJSON writes payload as JSON to cmd.OutOrStdout.
func emitJSON(cmd *cobra.Command, payload any) error {
	return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
}

// fileExists reports whether the given path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
