package generator

import (
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/accounts"
)

type fakeNFTLister struct{ ids []string }

func (f fakeNFTLister) AccountNFTs(string) ([]string, error) { return f.ids, nil }

func TestRecordSuccess_PopulatesTrackers(t *testing.T) {
	pool, _ := accounts.NewPool(0, 4)
	a0 := pool.All()[0].ClassicAddress
	a1 := pool.All()[1].ClassicAddress

	cases := []struct {
		name   string
		fields map[string]any
		seq    uint32
		count  func(*Tracker) int
		want   int
	}{
		{"EscrowCreate", map[string]any{"TransactionType": "EscrowCreate", "Account": a0}, 1, func(tr *Tracker) int { return tr.Escrows().Count() }, 1},
		{"OfferCreate", map[string]any{"TransactionType": "OfferCreate", "Account": a0}, 2, func(tr *Tracker) int { return tr.Offers().Count() }, 1},
		{"CheckCreate", map[string]any{"TransactionType": "CheckCreate", "Account": a0, "Destination": a1}, 3, func(tr *Tracker) int { return tr.Checks().Count() }, 1},
		{"PaymentChannelCreate", map[string]any{"TransactionType": "PaymentChannelCreate", "Account": a0, "Destination": a1}, 4, func(tr *Tracker) int { return tr.Channels().Count() }, 1},
		{"PermissionedDomainSet/create", map[string]any{"TransactionType": "PermissionedDomainSet", "Account": a0}, 5, func(tr *Tracker) int { return tr.Domains().Count() }, 1},
		{"MPTokenIssuanceCreate", map[string]any{"TransactionType": "MPTokenIssuanceCreate", "Account": a0}, 6, func(tr *Tracker) int { return tr.MPTs().Count() }, 1},
		{"OracleSet", map[string]any{"TransactionType": "OracleSet", "Account": a0, "OracleDocumentID": uint32(7)}, 7, func(tr *Tracker) int { return tr.Oracles().Count() }, 1},
		{"DIDSet", map[string]any{"TransactionType": "DIDSet", "Account": a0}, 8, func(tr *Tracker) int { return tr.DIDs().Count() }, 1},
		{"CredentialCreate", map[string]any{"TransactionType": "CredentialCreate", "Account": a0, "Subject": a1, "CredentialType": "AABB"}, 9, func(tr *Tracker) int { return tr.Credentials().Count() }, 1},
		{"AMMCreate", map[string]any{"TransactionType": "AMMCreate", "Account": a0, "Amount2": map[string]any{"currency": "USD", "issuer": a1, "value": "10"}}, 10, func(tr *Tracker) int { return tr.AMMs().Count() }, 1},
		{"NFTokenCreateOffer", map[string]any{"TransactionType": "NFTokenCreateOffer", "Account": a0, "NFTokenID": "00AA", "Flags": tfSellNFToken}, 11, func(tr *Tracker) int { return tr.NFTOffers().Count() }, 1},
	}

	for _, c := range cases {
		g := New(pool)
		g.RecordSuccess(&Tx{Fields: c.fields}, c.seq, nil)
		if got := c.count(g.Tracker()); got != c.want {
			t.Errorf("%s: tracker count = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestRecordSuccess_PermissionedDomainUpdateNotRecorded(t *testing.T) {
	pool, _ := accounts.NewPool(0, 4)
	g := New(pool)
	// An update (DomainID present) does not create a new object.
	g.RecordSuccess(&Tx{Fields: map[string]any{
		"TransactionType": "PermissionedDomainSet",
		"Account":         pool.All()[0].ClassicAddress,
		"DomainID":        "DEADBEEF",
	}}, 1, nil)
	if got := g.Tracker().Domains().Count(); got != 0 {
		t.Errorf("domain update should not be recorded, count = %d", got)
	}
}

func TestRecordSuccess_NFTokenMintUsesLister(t *testing.T) {
	pool, _ := accounts.NewPool(0, 4)
	g := New(pool)
	lister := fakeNFTLister{ids: []string{"ID1", "ID2", "ID1"}} // duplicate ignored
	g.RecordSuccess(&Tx{Fields: map[string]any{
		"TransactionType": "NFTokenMint",
		"Account":         pool.All()[0].ClassicAddress,
	}}, 1, lister)
	if got := g.Tracker().NFTs().Count(); got != 2 {
		t.Errorf("NFT count = %d, want 2", got)
	}

	// A nil lister leaves the NFT tracker untouched (no panic).
	g2 := New(pool)
	g2.RecordSuccess(&Tx{Fields: map[string]any{
		"TransactionType": "NFTokenMint",
		"Account":         pool.All()[0].ClassicAddress,
	}}, 1, nil)
	if got := g2.Tracker().NFTs().Count(); got != 0 {
		t.Errorf("NFT count with nil lister = %d, want 0", got)
	}
}
