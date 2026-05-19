package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type fakeDocker struct {
	builds []dockerBuildCall
	err    error
}

type dockerBuildCall struct {
	dir string
	tag string
}

func (f *fakeDocker) Build(_ context.Context, dir, tag string, _ io.Writer) error {
	f.builds = append(f.builds, dockerBuildCall{dir: dir, tag: tag})
	return f.err
}

func TestUp_RebuildGoXRPLFlag_InvokesDockerBuild(t *testing.T) {
	scenarioPath := absTestdata(t, "soak.yaml")
	withDiscoveryDir(t)
	srv := healthzServer(t)

	cli := fakeCLIForUp(t, "soak-mixed-3x2", srv.URL)
	docker := &fakeDocker{}
	deps := &upDeps{
		cli:        cli,
		httpClient: redirectClient(srv),
		docker:     docker,
	}

	_, _, err := runUpCmd(t, deps,
		"up", "--json", "--scenario", scenarioPath, "--wait-control", "5s",
		"--rebuild-goxrpl", "/tmp/some/worktree",
	)
	if err != nil {
		t.Fatalf("up: %v", err)
	}
	if len(docker.builds) != 1 {
		t.Fatalf("expected 1 docker build, got %d (%v)", len(docker.builds), docker.builds)
	}
	b := docker.builds[0]
	if b.dir != "/tmp/some/worktree" {
		t.Errorf("build dir: got %q want %q", b.dir, "/tmp/some/worktree")
	}
	if b.tag == "" || !strings.Contains(b.tag, "goxrpl") {
		t.Errorf("build tag should contain goxrpl, got %q", b.tag)
	}
}

func TestUp_RebuildRippledFlag(t *testing.T) {
	scenarioPath := absTestdata(t, "soak.yaml")
	withDiscoveryDir(t)
	srv := healthzServer(t)

	cli := fakeCLIForUp(t, "soak-mixed-3x2", srv.URL)
	docker := &fakeDocker{}
	deps := &upDeps{cli: cli, httpClient: redirectClient(srv), docker: docker}

	_, _, err := runUpCmd(t, deps,
		"up", "--json", "--scenario", scenarioPath, "--wait-control", "5s",
		"--rebuild-rippled", "/tmp/rippled-src",
	)
	if err != nil {
		t.Fatalf("up: %v", err)
	}
	if len(docker.builds) != 1 || docker.builds[0].dir != "/tmp/rippled-src" {
		t.Errorf("expected single rippled build at /tmp/rippled-src, got %v", docker.builds)
	}
}

func TestUp_RebuildFailureAborts(t *testing.T) {
	scenarioPath := absTestdata(t, "soak.yaml")
	withDiscoveryDir(t)
	srv := healthzServer(t)

	cli := fakeCLIForUp(t, "soak-mixed-3x2", srv.URL)
	docker := &fakeDocker{err: errors.New("compilation failed")}
	deps := &upDeps{cli: cli, httpClient: redirectClient(srv), docker: docker}

	_, _, err := runUpCmd(t, deps,
		"up", "--json", "--scenario", scenarioPath, "--wait-control", "5s",
		"--rebuild-goxrpl", "/tmp/broken",
	)
	if err == nil {
		t.Fatal("rebuild failure must abort the up command")
	}
	if !strings.Contains(err.Error(), "rebuild goxrpl") {
		t.Errorf("error should mention rebuild step, got: %v", err)
	}
	// kurtosis run must not have been invoked.
	for _, run := range cli.runs {
		if len(run) > 0 && run[0] == "run" {
			t.Errorf("kurtosis run should not be called when rebuild fails; got %v", cli.runs)
		}
	}
}

func TestUp_WithDashboardFlag_EnablesObservability(t *testing.T) {
	scenarioPath := absTestdata(t, "soak.yaml")
	withDiscoveryDir(t)
	srv := healthzServer(t)

	cli := fakeCLIForUp(t, "soak-mixed-3x2", srv.URL)
	deps := &upDeps{cli: cli, httpClient: redirectClient(srv), docker: &fakeDocker{}}

	_, _, err := runUpCmd(t, deps,
		"up", "--json", "--scenario", scenarioPath, "--wait-control", "5s",
		"--with-dashboard",
	)
	if err != nil {
		t.Fatalf("up: %v", err)
	}
	// The compiled args JSON must show observability enabled. Find the run call.
	var argsJSON string
	for _, run := range cli.runs {
		if len(run) >= 5 && run[0] == "run" {
			argsJSON = run[len(run)-1]
			break
		}
	}
	if argsJSON == "" {
		t.Fatalf("no kurtosis run call recorded; cli.runs=%v", cli.runs)
	}
	if !strings.Contains(argsJSON, `"enable_observability":true`) {
		t.Errorf("--with-dashboard should set enable_observability=true in compiled args; got: %s", argsJSON)
	}
}
