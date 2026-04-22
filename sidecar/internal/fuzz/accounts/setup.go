package accounts

import (
	"fmt"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// Setup* values govern the dense trust-line mesh seeded before fuzzing.
// They are package variables (not consts) so tests can shorten SetupLedgerWait.
var (
	SetupCurrency   = "USD"
	SetupLimit      = "1000000" // trust-line limit in IOU units
	SetupIOUFunding = "10000"   // amount of IOU each holder receives from each issuer
	SetupLedgerWait = 5 * time.Second
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
			res, err := client.SubmitTrustSet(
				wallets[i].Seed,
				wallets[i].ClassicAddress,
				SetupCurrency,
				wallets[j].ClassicAddress,
				SetupLimit,
			)
			if err != nil {
				return fmt.Errorf("trustset %s→%s: %w",
					wallets[i].ClassicAddress, wallets[j].ClassicAddress, err)
			}
			if res.EngineResult != "tesSUCCESS" && res.EngineResult != "terQUEUED" {
				return fmt.Errorf("trustset %s→%s: engine=%s (%s)",
					wallets[i].ClassicAddress, wallets[j].ClassicAddress,
					res.EngineResult, res.EngineResultMessage)
			}
		}
	}
	time.Sleep(SetupLedgerWait)

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
			res, err := client.SubmitPaymentIOU(
				wallets[j].Seed,
				wallets[j].ClassicAddress,
				wallets[i].ClassicAddress,
				amount,
			)
			if err != nil {
				return fmt.Errorf("iou payment %s→%s: %w",
					wallets[j].ClassicAddress, wallets[i].ClassicAddress, err)
			}
			if res.EngineResult != "tesSUCCESS" && res.EngineResult != "terQUEUED" {
				return fmt.Errorf("iou payment %s→%s: engine=%s (%s)",
					wallets[j].ClassicAddress, wallets[i].ClassicAddress,
					res.EngineResult, res.EngineResultMessage)
			}
		}
	}
	time.Sleep(SetupLedgerWait)
	return nil
}
