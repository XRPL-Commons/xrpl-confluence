package oracle

import (
	"fmt"
	"math/big"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// InvariantPoolBalance verifies that the sum of tracked account balances
// never strictly increases across consecutive CheckLedger calls.
//
// XRPL's XRP conservation invariant states that total XRP in the system only
// decreases (via burned fees). Restricting the check to the fuzzer's tracked
// accounts is a conservative proxy: the sum can only decrease from fees paid
// by our accounts, never increase. A strict increase indicates either (a) a
// rippled bug that mints XRP, (b) stale/inconsistent node data, or (c) our
// tracked set is wrong — all worth surfacing.
type InvariantPoolBalance struct {
	addresses []string
	lastSum   *big.Int
}

// NewInvariantPoolBalance tracks the given account addresses.
func NewInvariantPoolBalance(addresses []string) *InvariantPoolBalance {
	return &InvariantPoolBalance{addresses: addresses}
}

// CheckLedger sums current balances and compares to the previous sum. The
// first call always succeeds (establishes baseline). Subsequent calls fail if
// the sum strictly increased.
func (i *InvariantPoolBalance) CheckLedger(client *rpcclient.Client) error {
	sum := new(big.Int)
	for _, addr := range i.addresses {
		info, err := client.AccountInfo(addr)
		if err != nil {
			// Skip accounts that don't exist yet (e.g., early ledgers before
			// funding). A persistent lookup failure surfaces elsewhere.
			continue
		}
		b := new(big.Int)
		if _, ok := b.SetString(info.Balance, 10); !ok {
			return fmt.Errorf("%s: unparseable balance %q", addr, info.Balance)
		}
		sum.Add(sum, b)
	}
	if i.lastSum != nil && sum.Cmp(i.lastSum) > 0 {
		diff := new(big.Int).Sub(sum, i.lastSum)
		prev := i.lastSum.String()
		cur := sum.String()
		i.lastSum = new(big.Int).Set(sum)
		return fmt.Errorf("XRP conservation violated: prev=%s current=%s (increase of %s drops)", prev, cur, diff.String())
	}
	i.lastSum = new(big.Int).Set(sum)
	return nil
}

// LastSum returns the last observed sum, or nil if CheckLedger has never been
// called. For logging / corpus entries.
func (i *InvariantPoolBalance) LastSum() *big.Int {
	if i.lastSum == nil {
		return nil
	}
	return new(big.Int).Set(i.lastSum)
}
