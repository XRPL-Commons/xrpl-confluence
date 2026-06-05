package generator

import "testing"

// Golden values produced by goXRPL's own keylet package
// (goXRPL/keylet/keylet.go) for these exact addresses and sequence, ensuring
// the generator derives ledger-object IDs identically to the node under test.
// The two addresses are deterministic pool wallets 0 and 1 for fuzzSeed 0.
const (
	goldA0  = "rMWkjBqYeMj8nRmhJZFAf5Se7rauF63Gdg"
	goldA1  = "rh2QvwvY5AgUdm57wR1hDV4WwSQeUZTBWn"
	goldSeq = uint32(1234)

	goldCheck    = "6034C4DFF8E48278B6CFE07D8E75D0162EE85E5BF90FE21A0743DAFF27E5A19D"
	goldPayChan  = "210BD8FD61662E97EF77662A15CE143FDE7B17ACE24E852F4E2549294303C52D"
	goldNFTOffer = "DDDFB7E06AF9B7249714A3E24697B3C747AEA1BD47559001948C27801A347B4C"
	goldPermDom  = "E9A8D8AA5CA3AFCA6BC64A6668ABE3EC0F638F984DB6CE5CB2A48D444373CABC"
	goldMPTID    = "000004D2E10314906E63D0E23F80E033807F967BF490283C"
)

func TestKeylet_MatchesGoXRPL(t *testing.T) {
	cases := []struct {
		name string
		got  func() (string, error)
		want string
	}{
		{"Check", func() (string, error) { return checkID(goldA0, goldSeq) }, goldCheck},
		{"PayChannel", func() (string, error) { return payChannelID(goldA0, goldA1, goldSeq) }, goldPayChan},
		{"NFTokenOffer", func() (string, error) { return nftokenOfferID(goldA0, goldSeq) }, goldNFTOffer},
		{"PermissionedDomain", func() (string, error) { return permissionedDomainID(goldA0, goldSeq) }, goldPermDom},
		{"MPTokenIssuanceID", func() (string, error) { return mptIssuanceID(goldA0, goldSeq) }, goldMPTID},
	}
	for _, c := range cases {
		got, err := c.got()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s = %s, want %s", c.name, got, c.want)
		}
	}
}

func TestKeylet_RejectsBadAddress(t *testing.T) {
	if _, err := checkID("not-an-address", 1); err == nil {
		t.Fatal("expected error for invalid address")
	}
}
