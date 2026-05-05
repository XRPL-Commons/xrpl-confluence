package accounts

import (
	"math/rand/v2"
)

// Tier classifies a pool wallet by its account-state shape. Tier-specific
// setup runs once during SetupState (see ApplyAll); tier-aware generators
// can use Pool.PickTier to draw a wallet of a specific class.
type Tier int

const (
	// Rich: well above reserve, master key enabled, no signer list, no
	// regular key. Default for the M1 pool. Most generators target rich
	// accounts.
	Rich Tier = iota
	// AtReserve: balance trimmed to exactly the reserve_base. New tx that
	// would push the account below reserve must fail with tecINSUFF_RESERVE.
	AtReserve
	// Multisig: signer list installed (3 signers, quorum 2), master key
	// still enabled. Multisigned tx exercise SignerListSet / multisign paths.
	Multisig
	// RegularKey: a regular key set, master key disabled (asfDisableMaster).
	// Tx must be signed by the regular key, not the master.
	RegularKey
	// Blackholed: master key disabled, no regular key set. The account is
	// fully inert — every tx submitted from it must fail. Useful for testing
	// the failure-mode codepath (tefMASTER_DISABLED + tefNO_AUTH_REQUIRED).
	Blackholed
)

func (t Tier) String() string {
	switch t {
	case Rich:
		return "Rich"
	case AtReserve:
		return "AtReserve"
	case Multisig:
		return "Multisig"
	case RegularKey:
		return "RegularKey"
	case Blackholed:
		return "Blackholed"
	default:
		return "Unknown"
	}
}

// TierWeights expresses the desired distribution of wallets across tiers.
// Each field is a non-negative integer; the pool is partitioned proportionally.
// Weights of zero produce zero wallets in that tier.
type TierWeights struct {
	Rich       int
	AtReserve  int
	Multisig   int
	RegularKey int
	Blackholed int
}

// AssignTiers stamps each wallet in the pool with a Tier according to the
// weight distribution. Deterministic for a given (rng, weights). After the
// call, Pool.PickTier returns wallets from the requested tier.
func AssignTiers(pool *Pool, weights TierWeights, rng *rand.Rand) {
	wallets := pool.All()
	total := weights.Rich + weights.AtReserve + weights.Multisig + weights.RegularKey + weights.Blackholed
	if total == 0 {
		// Default: all rich.
		for _, w := range wallets {
			w.Tier = Rich
		}
		return
	}

	// Compute target counts: floor(weight/total * len), then distribute the
	// remainder to Rich (the safe default).
	targets := map[Tier]int{
		Rich:       weights.Rich * len(wallets) / total,
		AtReserve:  weights.AtReserve * len(wallets) / total,
		Multisig:   weights.Multisig * len(wallets) / total,
		RegularKey: weights.RegularKey * len(wallets) / total,
		Blackholed: weights.Blackholed * len(wallets) / total,
	}
	assigned := targets[Rich] + targets[AtReserve] + targets[Multisig] + targets[RegularKey] + targets[Blackholed]
	targets[Rich] += len(wallets) - assigned

	// Stamp wallets in order: indices 0..N-1 get tier T while targets[T] > 0.
	tiers := []Tier{Rich, AtReserve, Multisig, RegularKey, Blackholed}
	idx := 0
	for _, t := range tiers {
		for k := 0; k < targets[t]; k++ {
			wallets[idx].Tier = t
			idx++
		}
	}
	_ = rng // reserved for future shuffle; deterministic order is fine for now.
}
