package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/discovery"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/kurtosis"
)

func localScenario(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "network.yaml")
	if err := os.WriteFile(path, []byte(`apiVersion: confluence/v1
kind: Scenario
metadata:
  name: local-network
topology:
  rippled:
    count: 2
    image: custom-rippled:local
  goxrpl:
    count: 0
workload:
  kind: none
budget:
  duration: 8h
`), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestNetworkUpPublishesHostEndpoints(t *testing.T) {
	scenario := localScenario(t)
	withDiscoveryDir(t)
	srv := healthzServer(t)
	cli := fakeCLIForUp(t, "local-network", srv.URL)
	old := cli.next
	cli.next = func(args []string) (string, string, error) {
		if len(args) > 3 && args[0] == "service" && strings.HasPrefix(args[3], "rippled-") {
			return "UUID: node-id\nIP Address: 172.16.0.5\nPorts:\n  rpc: 5005/tcp -> http://127.0.0.1:53005\n  ws: 6006/tcp -> 127.0.0.1:53006\n  peer: 51235/tcp -> 127.0.0.1:53007\n", "", nil
		}
		return old(args)
	}
	d := &upDeps{cli: cli, httpClient: redirectClient(srv)}
	out, _, err := runUpCmd(t, d, "--enclave", "explicit-network", "up", "-f", scenario, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var cur discovery.Current
	if err := json.Unmarshal([]byte(out), &cur); err != nil {
		t.Fatal(err)
	}
	if cur.EnclaveID != "explicit-network" || len(cur.Nodes) != 2 {
		t.Fatalf("unexpected network: %+v", cur)
	}
	if cur.Nodes[0].RPC != "http://127.0.0.1:53005" || cur.Nodes[0].WS != "ws://127.0.0.1:53006" {
		t.Fatalf("must publish host-mapped endpoints: %+v", cur.Nodes)
	}
	for _, call := range cli.runs {
		if len(call) > 1 && call[0] == "enclave" && call[1] == "rm" {
			t.Fatalf("fresh network must not remove an enclave: %v", call)
		}
	}
	if _, err := discovery.ReadNetwork("explicit-network"); err != nil {
		t.Fatal(err)
	}
}

func TestNetworkExistingRequiresExplicitLifecycle(t *testing.T) {
	scenario := localScenario(t)
	withDiscoveryDir(t)
	cli := &fakeCLI{next: func(args []string) (string, string, error) {
		return "UUID Name Status\nabc local-network RUNNING\n", "", nil
	}}
	_, _, err := runUpCmd(t, &upDeps{cli: cli}, "up", "-f", scenario)
	if err == nil || !strings.Contains(err.Error(), "--resume") {
		t.Fatalf("expected explicit lifecycle error: %v", err)
	}
	if len(cli.runs) != 1 || strings.Join(cli.runs[0], " ") != "enclave ls" {
		t.Fatalf("must not mutate existing enclave: %v", cli.runs)
	}
}

func TestNetworkResumeFailureNeverRemovesEnclave(t *testing.T) {
	scenario := localScenario(t)
	withDiscoveryDir(t)
	if err := discovery.WriteNetwork("local-network", json.RawMessage(`{"rippled_count":2,"goxrpl_count":0}`)); err != nil {
		t.Fatal(err)
	}
	cli := &fakeCLI{next: func(args []string) (string, string, error) {
		if args[0] == "enclave" && args[1] == "ls" {
			return "UUID Name Status\nabc local-network RUNNING\n", "", nil
		}
		if args[0] == "run" {
			if !strings.Contains(args[len(args)-1], `"network_resume":true`) || !strings.Contains(args[len(args)-1], `"network_force_update":true`) {
				t.Errorf("resume must reload and recreate nodes: %v", args)
			}
			return "Error uploading package: EOF", "", errors.New("exit status 1")
		}
		t.Errorf("unexpected destructive retry call: %v", args)
		return "", "", nil
	}}
	_, _, err := runUpCmd(t, &upDeps{cli: cli}, "up", "-f", scenario, "--resume")
	if err == nil {
		t.Fatal("expected resume failure")
	}
	if len(cli.runs) != 2 {
		t.Fatalf("resume must make exactly one attempt: %v", cli.runs)
	}
}

func TestResumeRejectsGenesisChangeAllowsImages(t *testing.T) {
	old := json.RawMessage(`{"rippled_count":2,"goxrpl_count":0,"rippled_image":"a","network_config":{"network_id":10000,"amendments":[]}}`)
	next := json.RawMessage(`{"rippled_count":2,"goxrpl_count":0,"rippled_image":"b","network_config":{"amendments":[],"network_id":10000}}`)
	if err := compatibleNetwork(old, next); err != nil {
		t.Fatalf("image changes and object ordering are compatible: %v", err)
	}
	for _, altered := range []string{
		strings.Replace(string(next), `"network_id":10000`, `"network_id":10001`, 1),
		strings.Replace(string(next), `"rippled_count":2`, `"rippled_count":3`, 1),
	} {
		if err := compatibleNetwork(old, json.RawMessage(altered)); err == nil {
			t.Fatalf("expected reset requirement: %s", altered)
		}
	}
}

func TestNodeEndpointMissingPorts(t *testing.T) {
	_, err := nodeEndpoint(&kurtosis.ServiceInfo{Name: "rippled-0"})
	if err == nil {
		t.Fatal("missing endpoints must fail discovery")
	}
}

func TestNetworkResetRemovesExistingEnclave(t *testing.T) {
	scenario := localScenario(t)
	withDiscoveryDir(t)
	if err := discovery.Write(&discovery.Current{EnclaveID: "local-network"}); err != nil {
		t.Fatal(err)
	}
	cli := &fakeCLI{next: func(args []string) (string, string, error) {
		if args[0] == "enclave" && args[1] == "ls" {
			return "UUID Name Status\nabc local-network RUNNING\n", "", nil
		}
		if args[0] == "run" {
			return "", "invalid image", fmt.Errorf("exit 1")
		}
		return "", "", nil
	}}
	_, _, err := runUpCmd(t, &upDeps{cli: cli}, "up", "-f", scenario, "--reset")
	if err == nil {
		t.Fatal("expected failing image")
	}
	if len(cli.runs) != 3 || strings.Join(cli.runs[1], " ") != "enclave rm -f local-network" {
		t.Fatalf("explicit reset must remove before running: %v", cli.runs)
	}
	if _, err := discovery.Read(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed startup must not retain stale endpoints: %v", err)
	}
}

func TestNetworkResumeMissingEnclave(t *testing.T) {
	scenario := localScenario(t)
	withDiscoveryDir(t)
	cli := &fakeCLI{}
	_, _, err := runUpCmd(t, &upDeps{cli: cli}, "up", "-f", scenario, "--resume")
	if err == nil || !strings.Contains(err.Error(), "missing enclave") {
		t.Fatalf("expected missing enclave error: %v", err)
	}
	if len(cli.runs) != 1 {
		t.Fatalf("must not run or delete: %v", cli.runs)
	}
}

func TestEndpointsRefreshesMappedPorts(t *testing.T) {
	withDiscoveryDir(t)
	if err := discovery.Write(&discovery.Current{EnclaveID: "network"}); err != nil {
		t.Fatal(err)
	}
	if err := discovery.WriteNetwork("network", json.RawMessage(`{"rippled_count":2,"goxrpl_count":0,"enable_control":false,"enable_dashboard":false}`)); err != nil {
		t.Fatal(err)
	}
	cli := &fakeCLI{next: func(args []string) (string, string, error) {
		return "UUID: node\nPorts:\n  rpc: 5005/tcp -> http://127.0.0.1:55005\n  ws: 6006/tcp -> 127.0.0.1:55006\n", "", nil
	}}
	root := newRootCmd()
	replaceSubCmd(root, newEndpointsCmdWith(cli))
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetArgs([]string{"endpoints", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var got discovery.Current
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Nodes) != 2 || got.Nodes[0].RPC != "http://127.0.0.1:55005" || got.ControlURL != "" {
		t.Fatalf("bad endpoints: %+v", got)
	}
	if len(cli.runs) != 2 {
		t.Fatalf("disabled services should not be inspected: %v", cli.runs)
	}
}

func TestNetworkFailedResetPreservesMetadata(t *testing.T) {
	scenario := localScenario(t)
	withDiscoveryDir(t)
	old := json.RawMessage(`{"rippled_count":3,"goxrpl_count":0}`)
	if err := discovery.WriteNetwork("local-network", old); err != nil {
		t.Fatal(err)
	}
	cli := &fakeCLI{next: func(args []string) (string, string, error) {
		if args[0] == "enclave" && args[1] == "ls" {
			return "UUID Name Status\nabc local-network RUNNING\n", "", nil
		}
		return "", "engine unavailable", errors.New("exit 1")
	}}
	_, _, err := runUpCmd(t, &upDeps{cli: cli}, "up", "-f", scenario, "--reset")
	if err == nil {
		t.Fatal("expected removal failure")
	}
	got, err := discovery.ReadNetwork("local-network")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, old) {
		t.Fatalf("failed reset changed metadata: %s", got)
	}
	if len(cli.runs) != 2 {
		t.Fatalf("must stop after removal failure: %v", cli.runs)
	}
}
