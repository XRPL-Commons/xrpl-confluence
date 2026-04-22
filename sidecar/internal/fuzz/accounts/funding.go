package accounts

import (
	"fmt"
	"strconv"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// Genesis credentials used for all standalone / devnet-style rippled starts.
const (
	GenesisAddress = "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"
	GenesisSecret  = "snoPBrXtMeMyMHUVTgbuqAfg1SUTb"
)

// FundFromGenesis sends `perAccountDrops` XRP (in drops) from the genesis
// account to every wallet in the pool. It fails fast on the first non-tesSUCCESS
// result — at funding time nothing else should be hitting the network.
//
// This intentionally uses the existing rpcclient.Client instead of xrpl-go:
// signing from the genesis seed is already covered by the rippled node and
// the funding path has been working in trafficgen. xrpl-go takes over for
// user-account-originated txs in the generator.
func FundFromGenesis(client *rpcclient.Client, pool *Pool, perAccountDrops uint64) error {
	amount := strconv.FormatUint(perAccountDrops, 10)
	for _, w := range pool.All() {
		res, err := client.SubmitPayment(GenesisSecret, GenesisAddress, w.ClassicAddress, amount)
		if err != nil {
			return fmt.Errorf("fund %s: %w", w.ClassicAddress, err)
		}
		if res.EngineResult != "tesSUCCESS" {
			return fmt.Errorf("fund %s: engine_result=%s (%s)",
				w.ClassicAddress, res.EngineResult, res.EngineResultMessage)
		}
	}
	return nil
}
