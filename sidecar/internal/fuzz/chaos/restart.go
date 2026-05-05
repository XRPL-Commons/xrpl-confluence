package chaos

import (
	"context"
	"fmt"
)

// RestartEvent stops a container at Apply and starts it at Recover. The
// surrounding consensus cluster experiences a peer-down/peer-up window
// the soak loop's tx submissions exercise.
type RestartEvent struct {
	Runtime   NetworkRuntime
	Container string
}

// NewRestartEvent constructs a RestartEvent for the named container.
func NewRestartEvent(rt NetworkRuntime, container string) *RestartEvent {
	return &RestartEvent{Runtime: rt, Container: container}
}

func (e *RestartEvent) Name() string { return "restart:" + e.Container }

func (e *RestartEvent) Apply(ctx context.Context) error {
	if err := e.Runtime.Stop(ctx, e.Container); err != nil {
		return fmt.Errorf("stop %s: %w", e.Container, err)
	}
	return nil
}

func (e *RestartEvent) Recover(ctx context.Context) error {
	if err := e.Runtime.Start(ctx, e.Container); err != nil {
		return fmt.Errorf("start %s: %w", e.Container, err)
	}
	return nil
}
