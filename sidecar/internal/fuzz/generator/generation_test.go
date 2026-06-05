package generator

import (
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/accounts"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
)

// expectedNewTypes is every transaction type added beyond the original nine.
// Keeping the list explicit guards against a builder file losing its init()
// Register call (which would silently drop the type from the fuzzer).
var expectedNewTypes = []string{
	"OfferCancel",
	"AccountDelete",
	"SignerListSet",
	"DepositPreauth",
	"CheckCreate", "CheckCash", "CheckCancel",
	"PaymentChannelCreate", "PaymentChannelFund", "PaymentChannelClaim",
	"NFTokenMint", "NFTokenBurn", "NFTokenCreateOffer", "NFTokenAcceptOffer", "NFTokenCancelOffer", "NFTokenModify",
	"Clawback",
	"AMMCreate", "AMMDeposit", "AMMWithdraw", "AMMVote", "AMMBid", "AMMDelete", "AMMClawback",
	"DIDSet", "DIDDelete",
	"OracleSet", "OracleDelete",
	"CredentialCreate", "CredentialAccept", "CredentialDelete",
	"PermissionedDomainSet", "PermissionedDomainDelete",
	"MPTokenIssuanceCreate", "MPTokenAuthorize", "MPTokenIssuanceSet", "MPTokenIssuanceDestroy",
	"LedgerStateFix",
}

// seedAllTrackers records one object in every sub-tracker so that the reference
// tx types (which gate on tracker contents) are all buildable.
func seedAllTrackers(g *Generator, pool *accounts.Pool) {
	a0 := pool.All()[0].ClassicAddress
	a1 := pool.All()[1].ClassicAddress
	tr := g.Tracker()
	tr.Escrows().Record(a0, 9)
	tr.Offers().Record(a0, 10)
	tr.Checks().Record(a0, a1, 11)
	tr.Channels().Record(a0, a1, 12)
	tr.Domains().Record(a0, 13)
	tr.MPTs().Record(a0, 14)
	tr.Oracles().Record(a0, 5)
	tr.DIDs().Record(a0)
	tr.Credentials().Record(a0, a1, "AABBCCDD")
	tr.AMMs().Record(a0, "USD", a1)
	tr.NFTs().Record(a0, "000800000000000000000000000000000000000000000000000000000000000A")
	tr.NFTOffers().Record(a0, "000800000000000000000000000000000000000000000000000000000000000A", true, 15)
}

func TestAllExpectedTypesRegistered(t *testing.T) {
	candidateMu.RLock()
	defer candidateMu.RUnlock()
	for _, name := range expectedNewTypes {
		if _, ok := candidates[name]; !ok {
			t.Errorf("transaction type %q is not registered", name)
		}
	}
}

// TestEveryCandidateBuildsWellFormed builds each registered candidate (with all
// trackers seeded) and asserts the core invariants every fuzzer tx must hold:
// the right TransactionType, an Account drawn from the pool, and a Secret that
// actually signs for that Account.
func TestEveryCandidateBuildsWellFormed(t *testing.T) {
	pool, err := accounts.NewPool(0, 5)
	if err != nil {
		t.Fatal(err)
	}
	g := New(pool)
	seedAllTrackers(g, pool)
	r := corpus.NewRNG(99).Rand()

	seedByAddr := map[string]string{}
	for _, w := range pool.All() {
		seedByAddr[w.ClassicAddress] = w.Seed
	}

	candidateMu.RLock()
	snapshot := make([]CandidateTx, 0, len(candidates))
	for _, c := range candidates {
		snapshot = append(snapshot, c)
	}
	candidateMu.RUnlock()

	for _, c := range snapshot {
		tx, err := c.Build(g, r)
		if err != nil {
			t.Errorf("%s: build error: %v", c.TransactionType, err)
			continue
		}
		if tx.TransactionType() != c.TransactionType {
			t.Errorf("%s: TransactionType = %q", c.TransactionType, tx.TransactionType())
		}
		acct, _ := tx.Fields["Account"].(string)
		if acct == "" {
			t.Errorf("%s: missing Account", c.TransactionType)
			continue
		}
		wantSeed, inPool := seedByAddr[acct]
		if !inPool {
			t.Errorf("%s: Account %s not in pool", c.TransactionType, acct)
			continue
		}
		if tx.Secret != wantSeed {
			t.Errorf("%s: Secret does not sign for Account %s", c.TransactionType, acct)
		}
	}
}

// TestDeterministicBuilders confirms the deterministic (non-clock) builders
// produce identical output from identical RNG state, preserving replayability.
func TestDeterministicBuilders(t *testing.T) {
	pool, _ := accounts.NewPool(0, 5)
	g1, g2 := New(pool), New(pool)
	seedAllTrackers(g1, pool)
	seedAllTrackers(g2, pool)

	builders := map[string]func(*Generator, anyRand) (*Tx, error){
		"CheckCreate":           func(g *Generator, r anyRand) (*Tx, error) { return g.CheckCreate(r) },
		"NFTokenMint":           func(g *Generator, r anyRand) (*Tx, error) { return g.NFTokenMint(r) },
		"OfferCancel":           func(g *Generator, r anyRand) (*Tx, error) { return g.OfferCancel(r) },
		"MPTokenIssuanceCreate": func(g *Generator, r anyRand) (*Tx, error) { return g.MPTokenIssuanceCreate(r) },
	}
	for name, b := range builders {
		a, err := b(g1, corpus.NewRNG(7).Rand())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		c, err := b(g2, corpus.NewRNG(7).Rand())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if a.Secret != c.Secret || a.Fields["Account"] != c.Fields["Account"] {
			t.Errorf("%s not deterministic: %+v vs %+v", name, a.Fields, c.Fields)
		}
	}
}
