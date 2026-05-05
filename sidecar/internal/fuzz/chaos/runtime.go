// Package chaos schedules and applies disturbances (restart, partition,
// latency, amendment flip) on top of a running soak loop. Events use
// NetworkRuntime to reach into containers; the soak runner continues to
// submit txs and oracle-check the cluster's recovery.
package chaos

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// NetworkRuntime is the minimal interface chaos events need beyond the
// crash poller's ContainerRuntime: arbitrary command exec inside a
// container plus stop/start lifecycle. Tests inject a fake.
type NetworkRuntime interface {
	Exec(ctx context.Context, name string, cmd []string) ([]byte, error)
	Stop(ctx context.Context, name string) error
	Start(ctx context.Context, name string) error
}

// DockerNetworkRuntime implements NetworkRuntime against the local Docker
// daemon, using the same client-construction path as crash.DockerRuntime.
type DockerNetworkRuntime struct {
	cli *client.Client
}

// NewDockerNetworkRuntime dials the local daemon and pings it to fail fast
// when the socket is absent.
func NewDockerNetworkRuntime() (*DockerNetworkRuntime, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := cli.Ping(ctx); err != nil {
		_ = cli.Close()
		return nil, fmt.Errorf("docker ping: %w", err)
	}
	return &DockerNetworkRuntime{cli: cli}, nil
}

// Close releases the Docker client.
func (d *DockerNetworkRuntime) Close() error { return d.cli.Close() }

// Exec runs cmd inside the named container and returns combined stdout+stderr.
func (d *DockerNetworkRuntime) Exec(ctx context.Context, name string, cmd []string) ([]byte, error) {
	cid, err := d.resolveID(ctx, name)
	if err != nil {
		return nil, err
	}
	resp, err := d.cli.ContainerExecCreate(ctx, cid, types.ExecConfig{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return nil, fmt.Errorf("exec create: %w", err)
	}
	att, err := d.cli.ContainerExecAttach(ctx, resp.ID, types.ExecStartCheck{})
	if err != nil {
		return nil, fmt.Errorf("exec attach: %w", err)
	}
	defer att.Close()
	var out, errBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&out, &errBuf, att.Reader); err != nil {
		return nil, fmt.Errorf("exec read: %w", err)
	}
	insp, err := d.cli.ContainerExecInspect(ctx, resp.ID)
	if err != nil {
		return nil, fmt.Errorf("exec inspect: %w", err)
	}
	if insp.ExitCode != 0 {
		return nil, fmt.Errorf("exec %s %s exited %d: %s",
			name, strings.Join(cmd, " "), insp.ExitCode, errBuf.String())
	}
	return out.Bytes(), nil
}

// Stop sends SIGTERM and waits for the daemon to confirm container exit
// (Docker's default 10s grace).
func (d *DockerNetworkRuntime) Stop(ctx context.Context, name string) error {
	cid, err := d.resolveID(ctx, name)
	if err != nil {
		return err
	}
	timeoutSecs := 10
	return d.cli.ContainerStop(ctx, cid, container.StopOptions{Timeout: &timeoutSecs})
}

// Start starts a previously-stopped container.
func (d *DockerNetworkRuntime) Start(ctx context.Context, name string) error {
	cid, err := d.resolveID(ctx, name)
	if err != nil {
		return err
	}
	return d.cli.ContainerStart(ctx, cid, container.StartOptions{})
}

func (d *DockerNetworkRuntime) resolveID(ctx context.Context, name string) (string, error) {
	insp, err := d.cli.ContainerInspect(ctx, name)
	if err != nil {
		return "", fmt.Errorf("inspect %s: %w", name, err)
	}
	return insp.ID, nil
}
