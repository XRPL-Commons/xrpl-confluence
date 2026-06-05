package generator

// RecordSuccess feeds the tracker after a transaction is successfully applied,
// so the "reference" tx types (OfferCancel, CheckCash, MPTokenAuthorize, …)
// become eligible in subsequent picks. The runner calls this once per
// successful submit with the validated Sequence the node assigned. It replaces
// the per-type inline bookkeeping the runners used to carry, keeping all
// create→reference knowledge in the generator package.
//
// `lister` discovers minted NFTokenIDs (which cannot be derived from
// account+sequence alone) via account_nfts; callers without an RPC client —
// e.g. unit tests — may pass nil to skip that lookup.

// NFTLister lists the NFTokenIDs an account currently owns. The rpcclient
// Client satisfies it; nil disables NFT discovery.
type NFTLister interface {
	AccountNFTs(account string) ([]string, error)
}

// RecordSuccess updates the tracker from a just-applied transaction.
func (g *Generator) RecordSuccess(tx *Tx, sequence uint32, lister NFTLister) {
	if tx == nil {
		return
	}
	account := fieldString(tx.Fields, "Account")
	if account == "" {
		return
	}
	t := g.tracker

	switch tx.TransactionType() {
	case "EscrowCreate":
		if sequence > 0 {
			t.Escrows().Record(account, sequence)
		}
	case "OfferCreate":
		if sequence > 0 {
			t.Offers().Record(account, sequence)
		}
	case "CheckCreate":
		if sequence > 0 {
			t.Checks().Record(account, fieldString(tx.Fields, "Destination"), sequence)
		}
	case "PaymentChannelCreate":
		if sequence > 0 {
			t.Channels().Record(account, fieldString(tx.Fields, "Destination"), sequence)
		}
	case "PermissionedDomainSet":
		// Only a create (no DomainID) makes a new object whose ID we can derive.
		if sequence > 0 && fieldString(tx.Fields, "DomainID") == "" {
			t.Domains().Record(account, sequence)
		}
	case "MPTokenIssuanceCreate":
		if sequence > 0 {
			t.MPTs().Record(account, sequence)
		}
	case "OracleSet":
		if id, ok := fieldUint32(tx.Fields, "OracleDocumentID"); ok {
			t.Oracles().Record(account, id)
		}
	case "DIDSet":
		t.DIDs().Record(account)
	case "CredentialCreate":
		t.Credentials().Record(account, fieldString(tx.Fields, "Subject"), fieldString(tx.Fields, "CredentialType"))
	case "AMMCreate":
		if cur, iss, ok := fieldIOU(tx.Fields, "Amount2"); ok {
			t.AMMs().Record(account, cur, iss)
		}
	case "NFTokenMint":
		g.recordMintedNFTs(account, lister)
	case "NFTokenCreateOffer":
		if sequence > 0 {
			sell := false
			if f, ok := fieldUint32(tx.Fields, "Flags"); ok {
				sell = f&tfSellNFToken != 0
			}
			t.NFTOffers().Record(account, fieldString(tx.Fields, "NFTokenID"), sell, sequence)
		}
	}
}

// recordMintedNFTs queries account_nfts for the minter and records any newly
// seen NFTokenIDs. A nil lister or RPC error simply leaves the NFT tracker
// unchanged (Burn/CreateOffer stay ineligible until a mint is observed).
func (g *Generator) recordMintedNFTs(owner string, lister NFTLister) {
	if lister == nil {
		return
	}
	ids, err := lister.AccountNFTs(owner)
	if err != nil {
		return
	}
	for _, id := range ids {
		g.tracker.NFTs().Record(owner, id)
	}
}

// fieldString returns fields[key] as a string, or "" if absent/not a string.
func fieldString(fields map[string]any, key string) string {
	if s, ok := fields[key].(string); ok {
		return s
	}
	return ""
}

// fieldUint32 returns fields[key] as a uint32. Generated txs use uint32 for
// numeric fields; JSON round-trips (replay) may surface float64.
func fieldUint32(fields map[string]any, key string) (uint32, bool) {
	switch v := fields[key].(type) {
	case uint32:
		return v, true
	case int:
		return uint32(v), true
	case float64:
		return uint32(v), true
	default:
		return 0, false
	}
}

// fieldIOU extracts (currency, issuer) from an IOU amount object field.
func fieldIOU(fields map[string]any, key string) (currency, issuer string, ok bool) {
	m, isMap := fields[key].(map[string]any)
	if !isMap {
		return "", "", false
	}
	currency, _ = m["currency"].(string)
	issuer, _ = m["issuer"].(string)
	if currency == "" || issuer == "" {
		return "", "", false
	}
	return currency, issuer, true
}
