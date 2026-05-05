package chaos

import (
	"context"
	"testing"
)

func TestRestartEvent_StopsThenStarts(t *testing.T) {
	rt := &fakeRuntime{}
	e := NewRestartEvent(rt, "goxrpl-0")
	if e.Name() != "restart:goxrpl-0" {
		t.Fatalf("name = %q", e.Name())
	}
	if err := e.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"stop:goxrpl-0", "start:goxrpl-0"}
	if len(rt.calls) != 2 || rt.calls[0] != want[0] || rt.calls[1] != want[1] {
		t.Errorf("calls = %v, want %v", rt.calls, want)
	}
}
