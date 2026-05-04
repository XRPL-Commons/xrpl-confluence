package crash

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

// DockerRuntime implements ContainerRuntime against a local Docker daemon
// reachable via the host's UNIX socket. The sidecar mounts /var/run/docker.sock
// (see Phase B Task B5).
type DockerRuntime struct {
	cli *client.Client
}

// NewDockerRuntime dials the local Docker daemon. Caller closes via Close().
func NewDockerRuntime() (*DockerRuntime, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &DockerRuntime{cli: cli}, nil
}

// Close releases the underlying Docker client.
func (d *DockerRuntime) Close() error { return d.cli.Close() }

// ListByLabel returns container names (with leading "/" stripped) whose
// label matches key=val.
func (d *DockerRuntime) ListByLabel(ctx context.Context, key, val string) ([]string, error) {
	args := filters.NewArgs()
	args.Add("label", fmt.Sprintf("%s=%s", key, val))
	cs, err := d.cli.ContainerList(ctx, container.ListOptions{All: true, Filters: args})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		for _, n := range c.Names {
			out = append(out, strings.TrimPrefix(n, "/"))
		}
	}
	return out, nil
}

// Inspect returns the container's running flag and last exit code.
func (d *DockerRuntime) Inspect(ctx context.Context, name string) (bool, int, error) {
	info, err := d.cli.ContainerInspect(ctx, name)
	if err != nil {
		return false, 0, err
	}
	return info.State.Running, info.State.ExitCode, nil
}

// TailLogs returns up to lines log lines (combined stdout+stderr) ending at
// the current container tip.
func (d *DockerRuntime) TailLogs(ctx context.Context, name string, lines int) ([]string, error) {
	rdr, err := d.cli.ContainerLogs(ctx, name, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       fmt.Sprintf("%d", lines),
	})
	if err != nil {
		return nil, err
	}
	defer rdr.Close()
	var out []string
	scanner := bufio.NewScanner(rdr)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		// Docker multiplexed streams have an 8-byte header per frame. The
		// header bytes are non-printable; strip the leading 8 bytes if the
		// line starts with them.
		line := scanner.Text()
		if len(line) > 8 && line[0] < 0x20 {
			line = line[8:]
		}
		out = append(out, line)
	}
	return out, scanner.Err()
}

// SendSignal posts a kill signal (e.g. "QUIT", "TERM") to a container.
func (d *DockerRuntime) SendSignal(ctx context.Context, name, sig string) error {
	return d.cli.ContainerKill(ctx, name, sig)
}
