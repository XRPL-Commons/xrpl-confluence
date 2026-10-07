package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type cleanupCLI struct {
	calls int
	err   error
}

func (c *cleanupCLI) Run(ctx context.Context, args []string, _ io.Reader, _, _ io.Writer) error {
	c.calls++
	c.err = ctx.Err()
	return c.err
}

func TestRunTimeoutStillCleansUpWithLiveContext(t *testing.T) {
	withDiscoveryDir(t)
	controlURL, teardown := newRunServer(t)
	defer teardown()
	path := filepath.Join(t.TempDir(), "scenario.yaml")
	if err := os.WriteFile(path, []byte(strings.Replace(minimalScenarioYAML, "100ms", "1h", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	up, _ := newRunCmdDeps(t, controlURL)
	cleanup := &cleanupCLI{}
	_, _, err := runRunCmd(t, up, &downDeps{cli: cleanup}, "run", path, "--timeout=200ms", "--tear-down-first=false")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected timeout: %v", err)
	}
	if cleanup.calls != 1 || cleanup.err != nil {
		t.Fatalf("cleanup must use a live context after timeout: %+v", cleanup)
	}
}

func TestRunNonePointsToPersistentUp(t *testing.T) {
	path := localScenario(t)
	withDiscoveryDir(t)
	cli := &fakeCLI{}
	_, _, err := runRunCmd(t, &upDeps{cli: cli}, &downDeps{cli: cli}, "run", path)
	if err == nil || !strings.Contains(err.Error(), "confluence up") || len(cli.runs) != 0 {
		t.Fatalf("network-only run must explain the persistent workflow before mutations: %v, %v", err, cli.runs)
	}
}
