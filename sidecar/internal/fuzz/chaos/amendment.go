package chaos

import (
	"context"
	"fmt"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// AmendmentFlipEvent toggles a single rippled validator's vote on a
// named amendment. Apply votes yes (vetoed=false); Recover votes no.
// The targeted node must have admin RPC access — confluence's
// configuration grants admin to 0.0.0.0 on the test network.
type AmendmentFlipEvent struct {
	Client      *rpcclient.Client
	FeatureName string
}

// NewAmendmentFlipEvent constructs the event against the given client.
func NewAmendmentFlipEvent(c *rpcclient.Client, feature string) *AmendmentFlipEvent {
	return &AmendmentFlipEvent{Client: c, FeatureName: feature}
}

func (e *AmendmentFlipEvent) Name() string {
	return fmt.Sprintf("amendment:%s", e.FeatureName)
}

func (e *AmendmentFlipEvent) Apply(ctx context.Context) error {
	_, err := e.Client.Call("feature", map[string]any{
		"feature": e.FeatureName,
		"vetoed":  false,
	})
	if err != nil {
		return fmt.Errorf("feature vote-yes %s: %w", e.FeatureName, err)
	}
	return nil
}

func (e *AmendmentFlipEvent) Recover(ctx context.Context) error {
	_, err := e.Client.Call("feature", map[string]any{
		"feature": e.FeatureName,
		"vetoed":  true,
	})
	if err != nil {
		return fmt.Errorf("feature vote-no %s: %w", e.FeatureName, err)
	}
	return nil
}
