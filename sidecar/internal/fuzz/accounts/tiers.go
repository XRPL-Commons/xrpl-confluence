package accounts

import (
	"fmt"
	"math/rand/v2"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
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

// reserveBaseDrops is the XRPL reserve_base (200 XRP). Hard-coded — matches
// the test network's reserve setting in topology.star.
const reserveBaseDrops = 200_000_000

// submitter is the minimal interface tier setup needs: tx submission +
// account_info lookup. The full *rpcclient.Client satisfies it; tests inject
// a stub.
type submitter interface {
	SubmitTxJSON(secret string, tx map[string]any) (*rpcclient.SubmitResult, error)
	AccountInfo(addr string) (*rpcclient.AccountInfoResult, error)
}

// setupAtReserve drains the wallet's balance to exactly reserve_base by
// sending the excess to the genesis address. After this, any tx that
// requires sending more drops than (balance - reserve - fee) must fail
// with tecINSUFF_RESERVE — exactly the boundary we want to exercise.
func setupAtReserve(s submitter, w *Wallet) error {
	info, err := s.AccountInfo(w.ClassicAddress)
	if err != nil {
		return fmt.Errorf("at-reserve: account_info %s: %w", w.ClassicAddress, err)
	}
	balance, err := parseDrops(info.Balance)
	if err != nil {
		return fmt.Errorf("at-reserve: parse balance %q: %w", info.Balance, err)
	}
	// Send everything except reserve_base + 100 drops fee buffer.
	excess := balance - reserveBaseDrops - 100
	if excess <= 0 {
		return nil
	}
	_, err = s.SubmitTxJSON(w.Seed, map[string]any{
		"TransactionType": "Payment",
		"Account":         w.ClassicAddress,
		"Destination":     GenesisAddress,
		"Amount":          fmt.Sprintf("%d", excess),
	})
	return err
}

// setupMultisig installs a 3-of-quorum-2 SignerListSet on w using the given
// signer wallets' classic addresses. Master key remains enabled so the wallet
// can still sign single-sig — multisig is in addition, not replacement.
func setupMultisig(s submitter, w *Wallet, signers []*Wallet) error {
	if len(signers) < 3 {
		return fmt.Errorf("multisig: need >= 3 signers, got %d", len(signers))
	}
	entries := make([]map[string]any, 0, 3)
	for _, sg := range signers[:3] {
		entries = append(entries, map[string]any{
			"SignerEntry": map[string]any{
				"Account":      sg.ClassicAddress,
				"SignerWeight": uint32(1),
			},
		})
	}
	_, err := s.SubmitTxJSON(w.Seed, map[string]any{
		"TransactionType": "SignerListSet",
		"Account":         w.ClassicAddress,
		"SignerQuorum":    uint32(2),
		"SignerEntries":   entries,
	})
	return err
}

// asfDisableMaster is the AccountSet flag that disables the master keypair.
// After it lands, the master seed can no longer sign tx — regular key only.
const asfDisableMaster = uint32(4)

// setupRegularKey installs a RegularKey then disables the master. ORDER
// MATTERS: disabling the master before installing a regular key would lock
// the account out forever.
func setupRegularKey(s submitter, w *Wallet, regKeyAddr string) error {
	if _, err := s.SubmitTxJSON(w.Seed, map[string]any{
		"TransactionType": "SetRegularKey",
		"Account":         w.ClassicAddress,
		"RegularKey":      regKeyAddr,
	}); err != nil {
		return fmt.Errorf("regkey: SetRegularKey %s: %w", w.ClassicAddress, err)
	}
	if _, err := s.SubmitTxJSON(w.Seed, map[string]any{
		"TransactionType": "AccountSet",
		"Account":         w.ClassicAddress,
		"SetFlag":         asfDisableMaster,
	}); err != nil {
		return fmt.Errorf("regkey: disable-master %s: %w", w.ClassicAddress, err)
	}
	return nil
}

// parseDrops parses an XRPL balance string (decimal drops as string) into int64.
func parseDrops(s string) (int64, error) {
	var n int64
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return 0, err
	}
	return n, nil
}

// setupBlackholed disables the master without setting a regular key. The
// account is now inert. Every subsequent tx from it must fail with
// tefMASTER_DISABLED (or similar).
func setupBlackholed(s submitter, w *Wallet) error {
	_, err := s.SubmitTxJSON(w.Seed, map[string]any{
		"TransactionType": "AccountSet",
		"Account":         w.ClassicAddress,
		"SetFlag":         asfDisableMaster,
	})
	return err
}

// ApplyAll walks the pool and runs the appropriate per-tier setup on each
// wallet. Rich tier is a no-op (default state). Errors short-circuit and
// are returned with the failing wallet's address attached.
func ApplyAll(s submitter, pool *Pool) error {
	wallets := pool.All()
	for _, w := range wallets {
		var err error
		switch w.Tier {
		case Rich:
			continue
		case AtReserve:
			err = setupAtReserve(s, w)
		case Multisig:
			signers := otherWallets(wallets, w, 3)
			if len(signers) < 3 {
				return fmt.Errorf("multisig %s: pool has < 3 other wallets", w.ClassicAddress)
			}
			err = setupMultisig(s, w, signers)
		case RegularKey:
			signers := otherWallets(wallets, w, 1)
			if len(signers) < 1 {
				return fmt.Errorf("regkey %s: pool has < 1 other wallet", w.ClassicAddress)
			}
			err = setupRegularKey(s, w, signers[0].ClassicAddress)
		case Blackholed:
			err = setupBlackholed(s, w)
		}
		if err != nil {
			return fmt.Errorf("tier %s on %s: %w", w.Tier, w.ClassicAddress, err)
		}
	}
	return nil
}

// otherWallets returns up to n wallets from the pool whose ClassicAddress
// is not equal to skip's. Used to draw signers/regular-keys from siblings.
func otherWallets(wallets []*Wallet, skip *Wallet, n int) []*Wallet {
	out := make([]*Wallet, 0, n)
	for _, w := range wallets {
		if w.ClassicAddress == skip.ClassicAddress {
			continue
		}
		out = append(out, w)
		if len(out) == n {
			return out
		}
	}
	return out
}
