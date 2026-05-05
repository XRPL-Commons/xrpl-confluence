package accounts

import (
	"fmt"
	"strings"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// setupPollInterval is how often waitForValidation polls ServerInfo. Exposed
// to tests via package var; production runs use the default (750ms).
var setupPollInterval = 750 * time.Millisecond

// Setup* values govern the dense trust-line mesh seeded before fuzzing.
// They are package variables (not consts) so tests can shorten SetupLedgerWait.
var (
	SetupCurrency   = "USD"
	SetupLimit      = "1000000" // trust-line limit in IOU units
	SetupIOUFunding = "10000"   // amount of IOU each holder receives from each issuer
	SetupLedgerWait = 5 * time.Second
	// SetupMaxRetries is the number of times to retry a transient RPC error
	// (e.g. noCurrent, notReady, tooBusy) before giving up.
	SetupMaxRetries = 6
	// SetupRetryDelay is the wait between retries for transient errors.
	SetupRetryDelay = 2 * time.Second
)

// SetupState seeds a dense trust-line + IOU-balance mesh between every pair
// of distinct pool accounts. After this returns, any OfferCreate selling USD
// that a pool account received from another pool account has liquidity —
// eliminating the tecUNFUNDED_OFFER failures observed in the M1 smoke.
//
// Phases (each followed by a wait so the prior phase's txs are validated):
//  1. For every ordered pair (holder=i, issuer=j), i != j:
//     holder TrustSets issuer for USD up to SetupLimit.
//  2. For every ordered pair (holder=i, issuer=j), i != j:
//     issuer sends a Payment of SetupIOUFunding USD to holder.
//
// Totals for pool size n: 2 * n * (n-1) submissions. The linear submission
// pattern matches the existing FundFromGenesis helper.
func SetupState(client *rpcclient.Client, pool *Pool) error {
	wallets := pool.All()
	n := len(wallets)
	if n < 2 {
		return nil
	}

	// Phase 1: trust-line mesh.
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if i == j {
				continue
			}
			if err := retrySubmit(SetupMaxRetries, func() (*rpcclient.SubmitResult, error) {
				return client.SubmitTrustSet(
					wallets[i].Seed,
					wallets[i].ClassicAddress,
					SetupCurrency,
					wallets[j].ClassicAddress,
					SetupLimit,
				)
			}); err != nil {
				return fmt.Errorf("trustset %s→%s: %w",
					wallets[i].ClassicAddress, wallets[j].ClassicAddress, err)
			}
		}
	}
	if err := waitForValidation(client, 2, 2*time.Minute); err != nil {
		return fmt.Errorf("wait for trustset validation: %w", err)
	}

	// Phase 2: IOU funding. Issuer wallet[j] sends USD to holder wallet[i].
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if i == j {
				continue
			}
			amount := map[string]any{
				"currency": SetupCurrency,
				"issuer":   wallets[j].ClassicAddress,
				"value":    SetupIOUFunding,
			}
			if err := retrySubmit(SetupMaxRetries, func() (*rpcclient.SubmitResult, error) {
				return client.SubmitPaymentIOU(
					wallets[j].Seed,
					wallets[j].ClassicAddress,
					wallets[i].ClassicAddress,
					amount,
				)
			}); err != nil {
				return fmt.Errorf("iou payment %s→%s: %w",
					wallets[j].ClassicAddress, wallets[i].ClassicAddress, err)
			}
		}
	}
	if err := waitForValidation(client, 2, 2*time.Minute); err != nil {
		return fmt.Errorf("wait for iou payment validation: %w", err)
	}

	// Phase 3: tier-specific account configuration.
	if err := ApplyAll(client, pool); err != nil {
		return fmt.Errorf("apply tiers: %w", err)
	}
	if err := waitForValidation(client, 2, 2*time.Minute); err != nil {
		return fmt.Errorf("wait for tier setup validation: %w", err)
	}
	return nil
}

// retrySubmit calls fn up to maxRetries times, sleeping SetupRetryDelay between
// attempts, if the error looks transient (noCurrent, notReady, tooBusy).
// Non-transient errors and non-success engine results are returned immediately.
func retrySubmit(maxRetries int, fn func() (*rpcclient.SubmitResult, error)) error {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(SetupRetryDelay)
		}
		res, err := fn()
		if err != nil {
			if isTransientRPCError(err) {
				lastErr = err
				continue
			}
			return err
		}
		if res.EngineResult == "tesSUCCESS" || res.EngineResult == "terQUEUED" {
			return nil
		}
		return fmt.Errorf("engine=%s (%s)", res.EngineResult, res.EngineResultMessage)
	}
	return fmt.Errorf("transient error after %d retries: %w", maxRetries, lastErr)
}

// isTransientRPCError reports whether an error from the RPC layer is likely
// transient — i.e., the node is not yet ready (noCurrent, notReady, tooBusy).
func isTransientRPCError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "noCurrent") ||
		strings.Contains(msg, "notReady") ||
		strings.Contains(msg, "tooBusy")
}

// waitForValidation polls ServerInfo until validated_ledger.seq has advanced
// by at least `advance` from the first observed value. Returns an error if
// the timeout elapses first. Used between SetupState's two phases to ensure
// TrustSets are validated before IOU Payments reference them — a deterministic
// replacement for fixed time.Sleep(SetupLedgerWait) that adapts to slow nodes.
func waitForValidation(client *rpcclient.Client, advance int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	startSeq := -1
	for time.Now().Before(deadline) {
		info, err := client.ServerInfo()
		if err == nil {
			if startSeq < 0 {
				startSeq = info.Validated.Seq
			} else if info.Validated.Seq-startSeq >= advance {
				return nil
			}
		}
		time.Sleep(setupPollInterval)
	}
	return fmt.Errorf("timeout waiting for validated seq to advance by %d (start=%d)", advance, startSeq)
}
