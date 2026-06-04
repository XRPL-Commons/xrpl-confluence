package generator

import (
	mathrand "math/rand/v2"
	"sort"
	"strconv"
)

// itoa renders a uint32 for use in deterministic sort keys.
func itoa(u uint32) string { return strconv.FormatUint(uint64(u), 10) }

// This file holds the runtime sub-trackers the generator consults to build
// "reference" transaction types — those that must point at an object an
// earlier transaction created (OfferCancel→an offer, CheckCash→a check,
// MPTokenAuthorize→an issuance, …). Each sub-tracker mirrors the EscrowTracker
// style in tracker.go: append-only Records fed by the runner on successful
// submit, deterministic Pick* helpers consulted by builders. Like the escrow
// tracker, they never prune — referencing an already-consumed object yields a
// tec* result, which is itself a useful fuzz signal.

// pickDeterministic returns a uniformly random element of refs using a stable
// ordering (sorted by key) so a given RNG state always yields the same pick.
func pickDeterministic[T any](r *mathrand.Rand, refs []T, key func(T) string) (T, bool) {
	var zero T
	if len(refs) == 0 {
		return zero, false
	}
	snap := make([]T, len(refs))
	copy(snap, refs)
	sort.Slice(snap, func(i, j int) bool { return key(snap[i]) < key(snap[j]) })
	return snap[r.IntN(len(snap))], true
}

// --- Offers: (owner, sequence) → OfferCancel ---

// OfferRef identifies one offer by its creator + the sequence its OfferCreate
// was assigned (OfferCancel references OfferSequence, not an object ID).
type OfferRef struct {
	Owner    string
	Sequence uint32
}

// OfferTracker records OfferCreate sequences so OfferCancel can reference them.
type OfferTracker struct{ refs []OfferRef }

func newOfferTracker() *OfferTracker { return &OfferTracker{} }

// Record adds an offer reference (owner + creating sequence).
func (t *OfferTracker) Record(owner string, sequence uint32) {
	t.refs = append(t.refs, OfferRef{Owner: owner, Sequence: sequence})
}

// Count returns the number of recorded offers.
func (t *OfferTracker) Count() int { return len(t.refs) }

// Pick returns a deterministic random offer reference, ok=false if none.
func (t *OfferTracker) Pick(r *mathrand.Rand) (OfferRef, bool) {
	return pickDeterministic(r, t.refs, func(o OfferRef) string {
		return o.Owner + ":" + itoa(o.Sequence)
	})
}

// --- Checks: derived CheckID → CheckCash / CheckCancel ---

// CheckRef holds a check's derived object ID plus the parties that may act on
// it (destination cashes; owner or destination cancels).
type CheckRef struct {
	CheckID     string
	Owner       string
	Destination string
}

// CheckTracker records created checks (deriving the CheckID from owner+seq).
type CheckTracker struct{ refs []CheckRef }

func newCheckTracker() *CheckTracker { return &CheckTracker{} }

// Record derives the CheckID from (owner, sequence) and stores the reference.
// A non-decodable owner address is skipped rather than recorded.
func (t *CheckTracker) Record(owner, destination string, sequence uint32) {
	id, err := checkID(owner, sequence)
	if err != nil {
		return
	}
	t.refs = append(t.refs, CheckRef{CheckID: id, Owner: owner, Destination: destination})
}

// Count returns the number of recorded checks.
func (t *CheckTracker) Count() int { return len(t.refs) }

// Pick returns a deterministic random check, ok=false if none.
func (t *CheckTracker) Pick(r *mathrand.Rand) (CheckRef, bool) {
	return pickDeterministic(r, t.refs, func(c CheckRef) string { return c.CheckID })
}

// --- Payment channels: derived Channel → PaymentChannelFund / Claim ---

// ChannelRef holds a payment channel's derived object ID and its endpoints.
type ChannelRef struct {
	Channel     string
	Source      string
	Destination string
}

// ChannelTracker records created payment channels.
type ChannelTracker struct{ refs []ChannelRef }

func newChannelTracker() *ChannelTracker { return &ChannelTracker{} }

// Record derives the Channel ID from (source, destination, sequence).
func (t *ChannelTracker) Record(source, destination string, sequence uint32) {
	id, err := payChannelID(source, destination, sequence)
	if err != nil {
		return
	}
	t.refs = append(t.refs, ChannelRef{Channel: id, Source: source, Destination: destination})
}

// Count returns the number of recorded channels.
func (t *ChannelTracker) Count() int { return len(t.refs) }

// Pick returns a deterministic random channel, ok=false if none.
func (t *ChannelTracker) Pick(r *mathrand.Rand) (ChannelRef, bool) {
	return pickDeterministic(r, t.refs, func(c ChannelRef) string { return c.Channel })
}

// --- Permissioned domains: derived DomainID → PermissionedDomainDelete ---

// DomainRef holds a permissioned domain's derived object ID and its owner.
type DomainRef struct {
	DomainID string
	Owner    string
}

// DomainTracker records created permissioned domains.
type DomainTracker struct{ refs []DomainRef }

func newDomainTracker() *DomainTracker { return &DomainTracker{} }

// Record derives the DomainID from (owner, sequence).
func (t *DomainTracker) Record(owner string, sequence uint32) {
	id, err := permissionedDomainID(owner, sequence)
	if err != nil {
		return
	}
	t.refs = append(t.refs, DomainRef{DomainID: id, Owner: owner})
}

// Count returns the number of recorded domains.
func (t *DomainTracker) Count() int { return len(t.refs) }

// Pick returns a deterministic random domain, ok=false if none.
func (t *DomainTracker) Pick(r *mathrand.Rand) (DomainRef, bool) {
	return pickDeterministic(r, t.refs, func(d DomainRef) string { return d.DomainID })
}

// --- MPToken issuances: derived MPTokenIssuanceID → Authorize/Set/Destroy ---

// MPTIssuanceRef holds an MPT issuance ID and its issuer.
type MPTIssuanceRef struct {
	IssuanceID string
	Issuer     string
}

// MPTTracker records created MPToken issuances.
type MPTTracker struct{ refs []MPTIssuanceRef }

func newMPTTracker() *MPTTracker { return &MPTTracker{} }

// Record derives the MPTokenIssuanceID from (issuer, sequence).
func (t *MPTTracker) Record(issuer string, sequence uint32) {
	id, err := mptIssuanceID(issuer, sequence)
	if err != nil {
		return
	}
	t.refs = append(t.refs, MPTIssuanceRef{IssuanceID: id, Issuer: issuer})
}

// Count returns the number of recorded issuances.
func (t *MPTTracker) Count() int { return len(t.refs) }

// Pick returns a deterministic random issuance, ok=false if none.
func (t *MPTTracker) Pick(r *mathrand.Rand) (MPTIssuanceRef, bool) {
	return pickDeterministic(r, t.refs, func(m MPTIssuanceRef) string { return m.IssuanceID })
}

// --- Oracles: (owner, documentID) → OracleDelete ---

// OracleRef identifies a price oracle by its owner and chosen document ID.
type OracleRef struct {
	Owner      string
	DocumentID uint32
}

// OracleTracker records created price oracles.
type OracleTracker struct{ refs []OracleRef }

func newOracleTracker() *OracleTracker { return &OracleTracker{} }

// Record stores an oracle reference (owner + the document ID we chose).
func (t *OracleTracker) Record(owner string, documentID uint32) {
	t.refs = append(t.refs, OracleRef{Owner: owner, DocumentID: documentID})
}

// Count returns the number of recorded oracles.
func (t *OracleTracker) Count() int { return len(t.refs) }

// Pick returns a deterministic random oracle, ok=false if none.
func (t *OracleTracker) Pick(r *mathrand.Rand) (OracleRef, bool) {
	return pickDeterministic(r, t.refs, func(o OracleRef) string {
		return o.Owner + ":" + itoa(o.DocumentID)
	})
}

// --- DIDs: owner set → DIDDelete ---

// DIDTracker records accounts that have set a DID (one DID per account).
type DIDTracker struct {
	owners []string
	seen   map[string]struct{}
}

func newDIDTracker() *DIDTracker { return &DIDTracker{seen: map[string]struct{}{}} }

// Record adds an owner (deduplicated — a DID object is unique per account).
func (t *DIDTracker) Record(owner string) {
	if _, ok := t.seen[owner]; ok {
		return
	}
	t.seen[owner] = struct{}{}
	t.owners = append(t.owners, owner)
}

// Count returns the number of accounts with a DID.
func (t *DIDTracker) Count() int { return len(t.owners) }

// Pick returns a deterministic random DID owner, ok=false if none.
func (t *DIDTracker) Pick(r *mathrand.Rand) (string, bool) {
	return pickDeterministic(r, t.owners, func(s string) string { return s })
}

// --- Credentials: (issuer, subject, type) → CredentialAccept / Delete ---

// CredentialRef identifies a credential by issuer, subject and (hex) type.
type CredentialRef struct {
	Issuer         string
	Subject        string
	CredentialType string
}

// CredentialTracker records created credentials.
type CredentialTracker struct{ refs []CredentialRef }

func newCredentialTracker() *CredentialTracker { return &CredentialTracker{} }

// Record stores a credential reference.
func (t *CredentialTracker) Record(issuer, subject, credentialType string) {
	t.refs = append(t.refs, CredentialRef{Issuer: issuer, Subject: subject, CredentialType: credentialType})
}

// Count returns the number of recorded credentials.
func (t *CredentialTracker) Count() int { return len(t.refs) }

// Pick returns a deterministic random credential, ok=false if none.
func (t *CredentialTracker) Pick(r *mathrand.Rand) (CredentialRef, bool) {
	return pickDeterministic(r, t.refs, func(c CredentialRef) string {
		return c.Issuer + ":" + c.Subject + ":" + c.CredentialType
	})
}

// --- AMMs: (creator, asset2 currency+issuer) → Deposit/Withdraw/Vote/Bid/… ---

// AMMRef identifies an AMM pool by its second asset (the first is XRP) and the
// creator, who holds the LP tokens needed by Vote/Bid/Withdraw.
type AMMRef struct {
	Creator  string
	Currency string
	Issuer   string
}

// AMMTracker records created AMM pools (XRP / IOU).
type AMMTracker struct{ refs []AMMRef }

func newAMMTracker() *AMMTracker { return &AMMTracker{} }

// Record stores an AMM reference.
func (t *AMMTracker) Record(creator, currency, issuer string) {
	t.refs = append(t.refs, AMMRef{Creator: creator, Currency: currency, Issuer: issuer})
}

// Count returns the number of recorded AMMs.
func (t *AMMTracker) Count() int { return len(t.refs) }

// Pick returns a deterministic random AMM, ok=false if none.
func (t *AMMTracker) Pick(r *mathrand.Rand) (AMMRef, bool) {
	return pickDeterministic(r, t.refs, func(a AMMRef) string {
		return a.Creator + ":" + a.Currency + ":" + a.Issuer
	})
}

// --- NFTokens: (owner, NFTokenID) → Burn / CreateOffer ---

// NFTRef identifies a minted NFToken by its owner and NFTokenID. The runner
// discovers NFTokenIDs via account_nfts after a successful mint.
type NFTRef struct {
	Owner     string
	NFTokenID string
}

// NFTTracker records minted NFTokens, deduplicated by NFTokenID.
type NFTTracker struct {
	refs []NFTRef
	seen map[string]struct{}
}

func newNFTTracker() *NFTTracker { return &NFTTracker{seen: map[string]struct{}{}} }

// Record adds an (owner, NFTokenID) pair if not already known.
func (t *NFTTracker) Record(owner, nftokenID string) {
	if nftokenID == "" {
		return
	}
	if _, ok := t.seen[nftokenID]; ok {
		return
	}
	t.seen[nftokenID] = struct{}{}
	t.refs = append(t.refs, NFTRef{Owner: owner, NFTokenID: nftokenID})
}

// Count returns the number of recorded NFTokens.
func (t *NFTTracker) Count() int { return len(t.refs) }

// Pick returns a deterministic random NFToken, ok=false if none.
func (t *NFTTracker) Pick(r *mathrand.Rand) (NFTRef, bool) {
	return pickDeterministic(r, t.refs, func(n NFTRef) string { return n.NFTokenID })
}

// --- NFToken offers: derived NFTokenOfferID → AcceptOffer / CancelOffer ---

// NFTOfferRef holds an NFToken offer's derived object ID, its creator, the
// NFTokenID it targets, and whether it is a sell offer.
type NFTOfferRef struct {
	OfferID   string
	Owner     string
	NFTokenID string
	Sell      bool
}

// NFTOfferTracker records created NFToken offers.
type NFTOfferTracker struct{ refs []NFTOfferRef }

func newNFTOfferTracker() *NFTOfferTracker { return &NFTOfferTracker{} }

// Record derives the NFTokenOfferID from (owner, sequence) and stores it.
func (t *NFTOfferTracker) Record(owner, nftokenID string, sell bool, sequence uint32) {
	id, err := nftokenOfferID(owner, sequence)
	if err != nil {
		return
	}
	t.refs = append(t.refs, NFTOfferRef{OfferID: id, Owner: owner, NFTokenID: nftokenID, Sell: sell})
}

// Count returns the number of recorded NFToken offers.
func (t *NFTOfferTracker) Count() int { return len(t.refs) }

// Pick returns a deterministic random NFToken offer, ok=false if none.
func (t *NFTOfferTracker) Pick(r *mathrand.Rand) (NFTOfferRef, bool) {
	return pickDeterministic(r, t.refs, func(o NFTOfferRef) string { return o.OfferID })
}
