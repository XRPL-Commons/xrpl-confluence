package oracle

import (
	"context"
	"fmt"
	"time"
)

// WaitTxValidated blocks until every node reports `validated:true` for the
// given transaction hash, or until the per-node deadline elapses.
//
// Polls each node sequentially with backoff `interval` between rounds. Returns
// nil iff all nodes reported validated within the deadline; otherwise returns
// an error describing the lagging node or context cancellation.
func (o *Oracle) WaitTxValidated(ctx context.Context, hash string, timeout, interval time.Duration) error {
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	deadline := time.Now().Add(timeout)
	for _, n := range o.nodes {
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("wait %s on %s: timeout after %s", hash, n.Name, timeout)
			}
			res, err := n.Client.Tx(hash)
			if err == nil && res.Validated {
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(interval):
			}
		}
	}
	return nil
}
