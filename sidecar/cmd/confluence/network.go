package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/discovery"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/kurtosis"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/scenario"
	"github.com/spf13/cobra"
)

func (d *upDeps) checkNetworkLifecycle(ctx context.Context, s *api.Scenario, enclave string, o bootOptions) error {
	if o.Resume && o.Reset {
		return fmt.Errorf("--resume and --reset are mutually exclusive")
	}
	names, err := kurtosis.ListEnclaves(ctx, d.cli)
	if err != nil {
		return err
	}
	exists := false
	for _, name := range names {
		if name == enclave {
			exists = true
			break
		}
	}
	if o.Resume {
		if s.Topology.Goxrpl.Count != 0 {
			return fmt.Errorf("--resume currently requires a rippled-only network")
		}
		if !exists {
			return fmt.Errorf("cannot resume missing enclave %q; omit --resume to create it", enclave)
		}
		previous, err := discovery.ReadNetwork(enclave)
		if err != nil {
			return fmt.Errorf("cannot verify resume configuration: %w; run from the original working directory or use --reset", err)
		}
		next, err := scenario.Compile(s)
		if err != nil {
			return err
		}
		if err := compatibleNetwork(previous, next); err != nil {
			return err
		}
	} else if exists && !o.Reset {
		return fmt.Errorf("enclave %q already exists; use --resume to keep ledger data or --reset to delete it", enclave)
	}
	return nil
}

func compatibleNetwork(previous, next json.RawMessage) error {
	var oldArgs, newArgs map[string]any
	if err := json.Unmarshal(previous, &oldArgs); err != nil {
		return fmt.Errorf("invalid saved network: %w", err)
	}
	if err := json.Unmarshal(next, &newArgs); err != nil {
		return err
	}
	for _, field := range []string{"rippled_count", "goxrpl_count", "network_config", "enable_dashboard", "enable_control"} {
		if !reflect.DeepEqual(oldArgs[field], newArgs[field]) {
			return fmt.Errorf("cannot change %s while resuming; use --reset for topology, genesis, or service changes", field)
		}
	}
	return nil
}

func networkRunArgs(raw json.RawMessage, o bootOptions) (json.RawMessage, error) {
	var args map[string]any
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	timeout := o.WaitNetwork
	if timeout == 0 {
		timeout = 180 * time.Second
	}
	args["network_ready_timeout_seconds"] = int(timeout.Seconds())
	args["network_resume"] = o.Resume
	args["network_force_update"] = o.Resume
	return json.Marshal(args)
}

func serviceEnabled(raw json.RawMessage, field string) bool {
	var args map[string]any
	if json.Unmarshal(raw, &args) != nil {
		return true
	}
	enabled, exists := args[field].(bool)
	return !exists || enabled
}

func controlEnabled(raw json.RawMessage) bool { return serviceEnabled(raw, "enable_control") }

func (d *upDeps) discoverNetwork(ctx context.Context, cur *discovery.Current, rippled, goxrpl int, args json.RawMessage) error {
	for _, group := range []struct {
		name  string
		count int
	}{{"rippled", rippled}, {"goxrpl", goxrpl}} {
		for i := 0; i < group.count; i++ {
			name := fmt.Sprintf("%s-%d", group.name, i)
			svc, err := kurtosis.InspectService(ctx, d.cli, cur.EnclaveID, name)
			if err != nil {
				return err
			}
			node, err := nodeEndpoint(svc)
			if err != nil {
				return err
			}
			cur.Nodes = append(cur.Nodes, node)
		}
	}
	if serviceEnabled(args, "enable_dashboard") {
		svc, err := kurtosis.InspectService(ctx, d.cli, cur.EnclaveID, "dashboard")
		if err != nil {
			return err
		}
		cur.DashboardURL = serviceAddress(svc, "http", "http", 8080)
	}
	return nil
}

func serviceAddress(svc *kurtosis.ServiceInfo, port, scheme string, internal int) string {
	address := svc.PortURLs[port]
	if address == "" && svc.IPAddress != "" {
		address = fmt.Sprintf("%s:%d", svc.IPAddress, internal)
	}
	if address != "" && scheme != "" && !strings.Contains(address, "://") {
		address = scheme + "://" + address
	}
	return address
}

func nodeEndpoint(svc *kurtosis.ServiceInfo) (discovery.NodeEndpoint, error) {
	node := discovery.NodeEndpoint{
		Name: svc.Name,
		RPC:  serviceAddress(svc, "rpc", "http", 5005),
		WS:   serviceAddress(svc, "ws", "ws", 6006),
		Peer: serviceAddress(svc, "peer", "", 51235),
	}
	if node.RPC == "" || node.WS == "" {
		return node, fmt.Errorf("no published RPC/WS endpoints for %s", svc.Name)
	}
	return node, nil
}

func newEndpointsCmd() *cobra.Command {
	return newEndpointsCmdWith(kurtosis.NewExec())
}

func newEndpointsCmdWith(cli kurtosis.CLI) *cobra.Command {
	return &cobra.Command{
		Use:   "endpoints",
		Short: "Show current host RPC, WebSocket, and dashboard endpoints",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cur, err := discovery.Read()
			enclave, _ := cmd.Root().PersistentFlags().GetString("enclave")
			if enclave == "" {
				if err != nil {
					return fmt.Errorf("no current network: %w", err)
				}
				enclave = cur.EnclaveID
			}
			raw, err := discovery.ReadNetwork(enclave)
			if err != nil {
				return err
			}
			var counts struct {
				Rippled int `json:"rippled_count"`
				Goxrpl  int `json:"goxrpl_count"`
			}
			if err := json.Unmarshal(raw, &counts); err != nil {
				return err
			}
			cur = &discovery.Current{EnclaveID: enclave}
			d := &upDeps{cli: cli}
			if err := d.discoverNetwork(cmd.Context(), cur, counts.Rippled, counts.Goxrpl, raw); err != nil {
				return err
			}
			if controlEnabled(raw) {
				svc, err := kurtosis.InspectService(cmd.Context(), cli, enclave, "confluence-control")
				if err != nil {
					return err
				}
				cur.ControlURL = serviceAddress(svc, "http", "http", 8090)
			}
			if jsonMode(cmd) {
				return emitJSON(cmd, cur)
			}
			if cur.ControlURL != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Control: %s\n", cur.ControlURL)
			}
			return printEndpoints(cmd.OutOrStdout(), cur)
		},
	}
}

func printEndpoints(out io.Writer, cur *discovery.Current) error {
	if cur.DashboardURL != "" {
		if _, err := fmt.Fprintf(out, "Dashboard: %s\n", cur.DashboardURL); err != nil {
			return err
		}
	}
	for _, node := range cur.Nodes {
		if _, err := fmt.Fprintf(out, "%s: RPC %s | WS %s | peer %s\n", node.Name, node.RPC, node.WS, node.Peer); err != nil {
			return err
		}
	}
	return nil
}
