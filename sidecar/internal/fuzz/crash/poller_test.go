package crash

import (
	"context"
	"strings"
	"testing"
	"time"
)

// fakeRuntime implements ContainerRuntime for unit testing.
type fakeRuntime struct {
	containers map[string]containerSnapshot
}

type containerSnapshot struct {
	exitCode int
	running  bool
	logs     []string
}

func (f *fakeRuntime) ListByLabel(ctx context.Context, key, val string) ([]string, error) {
	out := []string{}
	for name := range f.containers {
		out = append(out, name)
	}
	return out, nil
}

func (f *fakeRuntime) Inspect(ctx context.Context, name string) (running bool, exitCode int, err error) {
	s, ok := f.containers[name]
	if !ok {
		return false, 0, errNotFound
	}
	return s.running, s.exitCode, nil
}

func (f *fakeRuntime) TailLogs(ctx context.Context, name string, lines int) ([]string, error) {
	s, ok := f.containers[name]
	if !ok {
		return nil, errNotFound
	}
	return s.logs, nil
}

func (f *fakeRuntime) SendSignal(ctx context.Context, name, sig string) error { return nil }

func TestPoller_DetectsCrash(t *testing.T) {
	rt := &fakeRuntime{containers: map[string]containerSnapshot{
		"goxrpl-0": {running: false, exitCode: 2, logs: []string{
			"foo",
			"panic: runtime error: nil pointer dereference",
			"goroutine 1 [running]:",
		}},
	}}
	got := []*Event{}
	p := NewPoller(rt, "fuzzer.role", "node", 4)
	p.OnCrash = func(e *Event) { got = append(got, e) }

	if err := p.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}
	if got[0].Kind != "go_panic" || got[0].Container != "goxrpl-0" || got[0].ExitCode != 2 {
		t.Fatalf("unexpected event: %+v", got[0])
	}

	// Second tick must not re-fire the same crash.
	got = got[:0]
	if err := p.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("re-fired stale event: %+v", got)
	}
}

func TestPoller_IgnoresHealthyContainers(t *testing.T) {
	rt := &fakeRuntime{containers: map[string]containerSnapshot{
		"rippled-0": {running: true, exitCode: 0, logs: []string{"ok"}},
	}}
	p := NewPoller(rt, "fuzzer.role", "node", 4)
	called := false
	p.OnCrash = func(*Event) { called = true }
	if err := p.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("OnCrash fired for healthy container")
	}
	_ = time.Second
}

func TestNewDockerRuntime_PingFailure(t *testing.T) {
	// Point at an unreachable TCP address so Ping returns an error quickly.
	t.Setenv("DOCKER_HOST", "tcp://127.0.0.1:1")
	_, err := NewDockerRuntime()
	if err == nil {
		t.Fatal("expected error when Docker daemon is unreachable, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "ping") && !strings.Contains(msg, "docker ping") {
		t.Fatalf("error %q does not mention ping", msg)
	}
}
