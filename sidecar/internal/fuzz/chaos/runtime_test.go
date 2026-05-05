package chaos

import (
	"context"
	"testing"
)

// fakeRuntime implements NetworkRuntime for unit tests.
type fakeRuntime struct {
	exec  func(ctx context.Context, name string, cmd []string) ([]byte, error)
	stop  func(ctx context.Context, name string) error
	start func(ctx context.Context, name string) error
	calls []string
}

func (f *fakeRuntime) Exec(ctx context.Context, name string, cmd []string) ([]byte, error) {
	f.calls = append(f.calls, "exec:"+name+":"+joinArgs(cmd))
	if f.exec != nil {
		return f.exec(ctx, name, cmd)
	}
	return nil, nil
}
func (f *fakeRuntime) Stop(ctx context.Context, name string) error {
	f.calls = append(f.calls, "stop:"+name)
	if f.stop != nil {
		return f.stop(ctx, name)
	}
	return nil
}
func (f *fakeRuntime) Start(ctx context.Context, name string) error {
	f.calls = append(f.calls, "start:"+name)
	if f.start != nil {
		return f.start(ctx, name)
	}
	return nil
}

func joinArgs(cmd []string) string {
	out := ""
	for i, c := range cmd {
		if i > 0 {
			out += " "
		}
		out += c
	}
	return out
}

func TestFakeRuntime_RecordsCalls(t *testing.T) {
	rt := &fakeRuntime{}
	if _, err := rt.Exec(context.Background(), "node-a", []string{"echo", "hi"}); err != nil {
		t.Fatal(err)
	}
	if err := rt.Stop(context.Background(), "node-a"); err != nil {
		t.Fatal(err)
	}
	if err := rt.Start(context.Background(), "node-a"); err != nil {
		t.Fatal(err)
	}
	want := []string{"exec:node-a:echo hi", "stop:node-a", "start:node-a"}
	if len(rt.calls) != len(want) {
		t.Fatalf("calls = %v, want %v", rt.calls, want)
	}
	for i, c := range rt.calls {
		if c != want[i] {
			t.Errorf("calls[%d] = %q, want %q", i, c, want[i])
		}
	}
}
