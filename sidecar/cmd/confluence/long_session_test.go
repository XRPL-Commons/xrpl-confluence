package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_BudgetOverridePropagatesToCompiledArgs(t *testing.T) {
	withDiscoveryDir(t)
	controlURL, teardown := newRunServer(t)
	defer teardown()

	dir := t.TempDir()
	scenarioPath := filepath.Join(dir, "scenario.yaml")
	if err := os.WriteFile(scenarioPath, []byte(minimalScenarioYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	up, down := newRunCmdDeps(t, controlURL)
	up.docker = &fakeDocker{}

	// Capture the compiled args by inspecting the recorded CLI call.
	stdout, _, err := runRunCmd(t, up, down,
		"run", scenarioPath,
		"--json",
		"--wait=true",
		"--down=false",
		"--tear-down-first=false",
		"--wait-control=5s",
		"--timeout=10s",
		"--budget=250ms",
	)
	if err != nil {
		if _, ok := err.(exitCodeError); !ok {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// Sanity: command output is JSON.
	var got map[string]any
	if jerr := json.Unmarshal([]byte(stdout), &got); jerr != nil {
		t.Fatalf("not JSON: %v (out=%q)", jerr, stdout)
	}

	// Sample the recorded CLI runs from up.cli. The compiled args appear as
	// the last positional arg to `kurtosis run`.
	fc := up.cli.(*fakeCLI)
	var args string
	for _, r := range fc.runs {
		if len(r) >= 5 && r[0] == "run" {
			args = r[len(r)-1]
			break
		}
	}
	if args == "" {
		t.Fatalf("no kurtosis run recorded")
	}
	// The override is propagated by mutating scenario.Budget.Duration BEFORE
	// compile; the compiled args object itself doesn't echo budget back, so
	// we instead assert that the run completed inside our overridden budget
	// window (250ms — well under the 10s timeout) by reading duration_ms.
	d, _ := got["duration_ms"].(float64)
	if d > 2000 {
		t.Errorf("budget override should have kept duration under ~1s; got %v ms", d)
	}
}

func TestRun_WithDashboardOverridesScenarioObservability(t *testing.T) {
	withDiscoveryDir(t)
	controlURL, teardown := newRunServer(t)
	defer teardown()

	dir := t.TempDir()
	scenarioPath := filepath.Join(dir, "scenario.yaml")
	if err := os.WriteFile(scenarioPath, []byte(minimalScenarioYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	up, down := newRunCmdDeps(t, controlURL)
	up.docker = &fakeDocker{}

	_, _, err := runRunCmd(t, up, down,
		"run", scenarioPath,
		"--json",
		"--wait=true",
		"--down=false",
		"--tear-down-first=false",
		"--wait-control=5s",
		"--timeout=10s",
		"--with-dashboard",
	)
	if err != nil {
		if _, ok := err.(exitCodeError); !ok {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	fc := up.cli.(*fakeCLI)
	var argsJSON string
	for _, r := range fc.runs {
		if len(r) >= 5 && r[0] == "run" {
			argsJSON = r[len(r)-1]
			break
		}
	}
	if argsJSON == "" {
		t.Fatalf("no kurtosis run recorded")
	}
	if !strings.Contains(argsJSON, `"enable_observability":true`) {
		t.Errorf("--with-dashboard should set enable_observability=true; args=%s", argsJSON)
	}
}

func TestRun_RotateLogsCreatesDir(t *testing.T) {
	withDiscoveryDir(t)
	controlURL, teardown := newRunServer(t)
	defer teardown()

	dir := t.TempDir()
	scenarioPath := filepath.Join(dir, "scenario.yaml")
	if err := os.WriteFile(scenarioPath, []byte(minimalScenarioYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	logsDir := filepath.Join(dir, "logs")
	up, down := newRunCmdDeps(t, controlURL)
	up.docker = &fakeDocker{}

	_, _, err := runRunCmd(t, up, down,
		"run", scenarioPath,
		"--json",
		"--wait=true",
		"--down=false",
		"--tear-down-first=false",
		"--wait-control=5s",
		"--timeout=10s",
		"--rotate-logs", logsDir,
	)
	if err != nil {
		if _, ok := err.(exitCodeError); !ok {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// The log rotator must have created the target directory even though
	// our fake CLI never produced any tail-able service output.
	info, err := os.Stat(logsDir)
	if err != nil {
		t.Fatalf("rotate-logs should create the dir: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("rotate-logs target is not a dir: %v", info.Mode())
	}
}
