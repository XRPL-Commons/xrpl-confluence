package generator

import (
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/accounts"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
)

// gatedTypes maps each amendment-gated tx type to true, derived from the
// registry, so the gating tests stay in sync with the builders.
func gatedTypes() map[string]bool {
	out := map[string]bool{}
	candidateMu.RLock()
	defer candidateMu.RUnlock()
	for name, c := range candidates {
		if len(c.RequiresAll) > 0 {
			out[name] = true
		}
	}
	return out
}

func allAmendments() []string {
	seen := map[string]struct{}{}
	candidateMu.RLock()
	defer candidateMu.RUnlock()
	for _, c := range candidates {
		for _, a := range c.RequiresAll {
			seen[a] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for a := range seen {
		out = append(out, a)
	}
	return out
}

// With no amendments enabled, PickTx must never return an amendment-gated type.
func TestPickTx_ExcludesGatedTypesWithoutAmendments(t *testing.T) {
	pool, _ := accounts.NewPool(0, 5)
	g := New(pool)
	seedAllTrackers(g, pool) // satisfy CanBuild so only amendment gating filters
	gated := gatedTypes()
	r := corpus.NewRNG(3).Rand()

	for i := 0; i < 2000; i++ {
		tx, err := g.PickTx(r, []string{})
		if err != nil {
			t.Fatal(err)
		}
		if gated[tx.TransactionType()] {
			t.Fatalf("gated type %q selected with no amendments enabled", tx.TransactionType())
		}
	}
}

// With every amendment enabled and trackers seeded, the gated types become
// reachable — confirm a representative sample actually appears.
func TestPickTx_IncludesGatedTypesWithAmendments(t *testing.T) {
	pool, _ := accounts.NewPool(0, 5)
	g := New(pool)
	seedAllTrackers(g, pool)
	r := corpus.NewRNG(3).Rand()

	seen := map[string]bool{}
	for i := 0; i < 6000; i++ {
		tx, err := g.PickTx(r, allAmendments())
		if err != nil {
			t.Fatal(err)
		}
		seen[tx.TransactionType()] = true
	}
	for _, want := range []string{"CheckCreate", "AMMCreate", "DIDSet", "MPTokenIssuanceCreate", "NFTokenMint"} {
		if !seen[want] {
			t.Errorf("gated type %q never selected despite amendment enabled", want)
		}
	}
}
