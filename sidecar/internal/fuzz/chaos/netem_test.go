package chaos

import (
	"context"
	"strings"
	"testing"
)

func TestLatencyEvent_AddsThenRemovesQdisc(t *testing.T) {
	rt := &fakeRuntime{}
	e := NewLatencyEvent(rt, "rippled-0", "eth0", 200)
	if !strings.HasPrefix(e.Name(), "latency:rippled-0") {
		t.Fatalf("name = %q", e.Name())
	}
	if err := e.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(rt.calls) != 2 {
		t.Fatalf("calls = %v", rt.calls)
	}
	if !strings.Contains(rt.calls[0], "tc qdisc add dev eth0 root netem delay 200ms") {
		t.Errorf("apply call = %q", rt.calls[0])
	}
	if !strings.Contains(rt.calls[1], "tc qdisc del dev eth0 root") {
		t.Errorf("recover call = %q", rt.calls[1])
	}
}

func TestPartitionEvent_AddsThenRemovesIptables(t *testing.T) {
	rt := &fakeRuntime{}
	e := NewPartitionEvent(rt, "rippled-0", "rippled-1")
	if e.Name() != "partition:rippled-0->rippled-1" {
		t.Fatalf("name = %q", e.Name())
	}
	if err := e.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(rt.calls) != 2 {
		t.Fatalf("calls = %v", rt.calls)
	}
	if !strings.Contains(rt.calls[0], "iptables -A OUTPUT -d rippled-1 -j DROP") {
		t.Errorf("apply call = %q", rt.calls[0])
	}
	if !strings.Contains(rt.calls[1], "iptables -D OUTPUT -d rippled-1 -j DROP") {
		t.Errorf("recover call = %q", rt.calls[1])
	}
}
