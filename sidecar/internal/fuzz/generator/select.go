package generator

import (
	"fmt"
	mathrand "math/rand/v2"
	"sort"
	"sync"
)

type anyRand = *mathrand.Rand

// CandidateTx describes one tx type available to the selector. Amendment-
// gated types list their required amendments; the selector excludes any type
// whose requirements are not all present in the live amendment set.
type CandidateTx struct {
	TransactionType string
	RequiresAll     []string
	Build           func(g *Generator, r anyRand) (*Tx, error)
}

var (
	candidateMu sync.RWMutex
	candidates  = map[string]CandidateTx{}
)

// Register adds a CandidateTx. Called from init() of each builder file.
func Register(c CandidateTx) {
	candidateMu.Lock()
	defer candidateMu.Unlock()
	if _, exists := candidates[c.TransactionType]; exists {
		panic(fmt.Sprintf("duplicate candidate tx type: %s", c.TransactionType))
	}
	candidates[c.TransactionType] = c
}

// test helpers — not exported to non-test callers.

// registerForTest registers a CandidateTx in the package-global registry
// for use by tests. Because the registry is process-global, tests using
// this helper (or the companion unregisterForTest) MUST NOT call
// t.Parallel() — parallel registration would race and/or panic on duplicate.
func registerForTest(c CandidateTx) { Register(c) }
func unregisterForTest(name string) func() {
	return func() {
		candidateMu.Lock()
		delete(candidates, name)
		candidateMu.Unlock()
	}
}

// PickTx selects a random CandidateTx eligible under the given amendment set
// and builds it. Returns an error only if no candidates are eligible.
func (g *Generator) PickTx(r anyRand, enabledAmendments []string) (*Tx, error) {
	enabled := make(map[string]struct{}, len(enabledAmendments))
	for _, a := range enabledAmendments {
		enabled[a] = struct{}{}
	}

	candidateMu.RLock()
	all := make([]CandidateTx, 0, len(candidates))
	for _, c := range candidates {
		all = append(all, c)
	}
	candidateMu.RUnlock()
	sort.Slice(all, func(i, j int) bool { return all[i].TransactionType < all[j].TransactionType })

	eligible := make([]CandidateTx, 0, len(all))
	for _, c := range all {
		if allIn(enabled, c.RequiresAll) {
			eligible = append(eligible, c)
		}
	}

	if len(eligible) == 0 {
		return nil, fmt.Errorf("no eligible tx types for current amendment set")
	}
	choice := eligible[r.IntN(len(eligible))]
	return choice.Build(g, r)
}

func allIn(set map[string]struct{}, keys []string) bool {
	for _, k := range keys {
		if _, ok := set[k]; !ok {
			return false
		}
	}
	return true
}
