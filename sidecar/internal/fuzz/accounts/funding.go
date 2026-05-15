package accounts

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// Genesis credentials used for all standalone / devnet-style rippled starts.
const (
	GenesisAddress = "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"
	GenesisSecret  = "snoPBrXtMeMyMHUVTgbuqAfg1SUTb"
)

// Tunables for the transient-retry loop. Package vars so tests can shorten
// the delay; production runs use the defaults.
var (
	FundMaxAttempts = 30
	FundRetryDelay  = 2 * time.Second
)

// FundFromGenesis sends `perAccountDrops` XRP (in drops) from the genesis
// account to every wallet in the pool. It accepts tesSUCCESS / terQUEUED and
// retries transient tel/ter results (e.g. telCAN_NOT_QUEUE_FULL) which
// rippled emits when the txQ is temporarily saturated by the funding burst.
func FundFromGenesis(client *rpcclient.Client, pool *Pool, perAccountDrops uint64) error {
	amount := strconv.FormatUint(perAccountDrops, 10)
	for _, w := range pool.All() {
		var lastErr error
		var lastRes *rpcclient.SubmitResult
		for attempt := 0; attempt < FundMaxAttempts; attempt++ {
			res, err := client.SubmitPayment(GenesisSecret, GenesisAddress, w.ClassicAddress, amount)
			if err != nil {
				lastErr = err
				time.Sleep(FundRetryDelay)
				continue
			}
			lastRes = res
			lastErr = nil
			if res.EngineResult == "tesSUCCESS" || res.EngineResult == "terQUEUED" {
				break
			}
			if isTransientFundResult(res.EngineResult) {
				time.Sleep(FundRetryDelay)
				continue
			}
			return fmt.Errorf("fund %s: engine_result=%s (%s)",
				w.ClassicAddress, res.EngineResult, res.EngineResultMessage)
		}
		if lastErr != nil {
			return fmt.Errorf("fund %s: %w", w.ClassicAddress, lastErr)
		}
		if lastRes != nil && lastRes.EngineResult != "tesSUCCESS" && lastRes.EngineResult != "terQUEUED" {
			return fmt.Errorf("fund %s: engine_result=%s (%s) after %d attempts",
				w.ClassicAddress, lastRes.EngineResult, lastRes.EngineResultMessage, FundMaxAttempts)
		}
	}
	return nil
}

func isTransientFundResult(code string) bool {
	if strings.HasPrefix(code, "tel") {
		return true
	}
	if strings.HasPrefix(code, "ter") {
		return true
	}
	return false
}
