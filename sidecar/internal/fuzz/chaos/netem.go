package chaos

import (
	"context"
	"fmt"
)

// LatencyEvent adds a `tc netem delay` qdisc on Apply and removes it on
// Recover. Requires iproute2 inside the target container; rippled
// containers ship it, the goXRPL distroless image does not.
type LatencyEvent struct {
	Runtime   NetworkRuntime
	Container string
	Iface     string
	DelayMs   int
}

// NewLatencyEvent builds a LatencyEvent for the named container/interface.
func NewLatencyEvent(rt NetworkRuntime, container, iface string, delayMs int) *LatencyEvent {
	return &LatencyEvent{Runtime: rt, Container: container, Iface: iface, DelayMs: delayMs}
}

func (e *LatencyEvent) Name() string {
	return fmt.Sprintf("latency:%s:%dms", e.Container, e.DelayMs)
}

func (e *LatencyEvent) Apply(ctx context.Context) error {
	cmd := []string{"tc", "qdisc", "add", "dev", e.Iface, "root", "netem", "delay",
		fmt.Sprintf("%dms", e.DelayMs)}
	_, err := e.Runtime.Exec(ctx, e.Container, cmd)
	return err
}

func (e *LatencyEvent) Recover(ctx context.Context) error {
	cmd := []string{"tc", "qdisc", "del", "dev", e.Iface, "root"}
	_, err := e.Runtime.Exec(ctx, e.Container, cmd)
	return err
}

// PartitionEvent drops outbound traffic from one container to one peer
// at Apply, removes the rule at Recover. Implements one-way partition;
// for symmetric partitions schedule two PartitionEvents in opposite
// directions on the same trigger step.
type PartitionEvent struct {
	Runtime NetworkRuntime
	From    string
	To      string
}

// NewPartitionEvent builds a PartitionEvent dropping `from`'s outbound
// traffic to `to` (DNS-resolved by the kernel inside the container).
func NewPartitionEvent(rt NetworkRuntime, from, to string) *PartitionEvent {
	return &PartitionEvent{Runtime: rt, From: from, To: to}
}

func (e *PartitionEvent) Name() string {
	return fmt.Sprintf("partition:%s->%s", e.From, e.To)
}

func (e *PartitionEvent) Apply(ctx context.Context) error {
	cmd := []string{"iptables", "-A", "OUTPUT", "-d", e.To, "-j", "DROP"}
	_, err := e.Runtime.Exec(ctx, e.From, cmd)
	return err
}

func (e *PartitionEvent) Recover(ctx context.Context) error {
	cmd := []string{"iptables", "-D", "OUTPUT", "-d", e.To, "-j", "DROP"}
	_, err := e.Runtime.Exec(ctx, e.From, cmd)
	return err
}
