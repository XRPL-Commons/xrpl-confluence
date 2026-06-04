package generator

import (
	mathrand "math/rand/v2"
	"sort"
)

// Tracker holds per-family runtime state so the generator can emit tx types
// that require referencing existing ledger objects (e.g. EscrowFinish needs
// an open escrow). The runner feeds the tracker on successful submits; the
// generator consults it when building.
type Tracker struct {
	escrows     *EscrowTracker
	offers      *OfferTracker
	checks      *CheckTracker
	channels    *ChannelTracker
	domains     *DomainTracker
	mpts        *MPTTracker
	oracles     *OracleTracker
	dids        *DIDTracker
	credentials *CredentialTracker
	amms        *AMMTracker
	nfts        *NFTTracker
	nftOffers   *NFTOfferTracker
}

// NewTracker returns a Tracker with every sub-tracker initialized.
func NewTracker() *Tracker {
	return &Tracker{
		escrows:     newEscrowTracker(),
		offers:      newOfferTracker(),
		checks:      newCheckTracker(),
		channels:    newChannelTracker(),
		domains:     newDomainTracker(),
		mpts:        newMPTTracker(),
		oracles:     newOracleTracker(),
		dids:        newDIDTracker(),
		credentials: newCredentialTracker(),
		amms:        newAMMTracker(),
		nfts:        newNFTTracker(),
		nftOffers:   newNFTOfferTracker(),
	}
}

// Escrows returns the escrow sub-tracker.
func (t *Tracker) Escrows() *EscrowTracker { return t.escrows }

// Offers returns the offer sub-tracker (OfferCancel).
func (t *Tracker) Offers() *OfferTracker { return t.offers }

// Checks returns the check sub-tracker (CheckCash / CheckCancel).
func (t *Tracker) Checks() *CheckTracker { return t.checks }

// Channels returns the payment-channel sub-tracker (Fund / Claim).
func (t *Tracker) Channels() *ChannelTracker { return t.channels }

// Domains returns the permissioned-domain sub-tracker (Delete).
func (t *Tracker) Domains() *DomainTracker { return t.domains }

// MPTs returns the MPToken-issuance sub-tracker (Authorize / Set / Destroy).
func (t *Tracker) MPTs() *MPTTracker { return t.mpts }

// Oracles returns the price-oracle sub-tracker (OracleDelete).
func (t *Tracker) Oracles() *OracleTracker { return t.oracles }

// DIDs returns the DID sub-tracker (DIDDelete).
func (t *Tracker) DIDs() *DIDTracker { return t.dids }

// Credentials returns the credential sub-tracker (Accept / Delete).
func (t *Tracker) Credentials() *CredentialTracker { return t.credentials }

// AMMs returns the AMM sub-tracker (Deposit / Withdraw / Vote / Bid / Delete).
func (t *Tracker) AMMs() *AMMTracker { return t.amms }

// NFTs returns the NFToken sub-tracker (Burn / CreateOffer).
func (t *Tracker) NFTs() *NFTTracker { return t.nfts }

// NFTOffers returns the NFToken-offer sub-tracker (Accept / Cancel).
func (t *Tracker) NFTOffers() *NFTOfferTracker { return t.nftOffers }

// EscrowRef identifies one escrow ledger object by its creator account +
// the Sequence the creating tx was assigned.
type EscrowRef struct {
	Owner    string
	Sequence uint32
}

// EscrowTracker records escrow creations so Finish/Cancel can reference them.
// It never removes records — even after Finish or Cancel, the sequence is
// still valid to reference (rippled returns tecNO_TARGET, which is a useful
// fuzz signal). Production-hardening would prune on observed Finish/Cancel.
type EscrowTracker struct {
	refs []EscrowRef
}

func newEscrowTracker() *EscrowTracker {
	return &EscrowTracker{refs: []EscrowRef{}}
}

// Record adds a new escrow reference. Callers must ensure Owner + Sequence
// correspond to an on-ledger EscrowCreate they have already submitted.
func (e *EscrowTracker) Record(owner string, sequence uint32) {
	e.refs = append(e.refs, EscrowRef{Owner: owner, Sequence: sequence})
}

// Count returns the number of recorded escrows.
func (e *EscrowTracker) Count() int {
	return len(e.refs)
}

// PickOpen returns a deterministic random escrow reference, or ok=false if
// none are tracked.
func (e *EscrowTracker) PickOpen(r *mathrand.Rand) (owner string, sequence uint32, ok bool) {
	if len(e.refs) == 0 {
		return "", 0, false
	}
	snap := make([]EscrowRef, len(e.refs))
	copy(snap, e.refs)
	sort.Slice(snap, func(i, j int) bool {
		if snap[i].Owner != snap[j].Owner {
			return snap[i].Owner < snap[j].Owner
		}
		return snap[i].Sequence < snap[j].Sequence
	})
	pick := snap[r.IntN(len(snap))]
	return pick.Owner, pick.Sequence, true
}
