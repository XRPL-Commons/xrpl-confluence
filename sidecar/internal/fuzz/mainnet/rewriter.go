package mainnet

import (
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/accounts"
)

// Rewriter substitutes mainnet addresses in a tx with pool-account addresses
// so the tx can be submitted against our private topology. Mapping is
// deterministic: the same mainnet address always maps to the same pool
// account within a single rewriter's lifetime.
type Rewriter struct {
	pool    *accounts.Pool
	cache   map[string]int // mainnet address → pool index
	counter int            // next pool slot to assign
}

// NewRewriter constructs a Rewriter over the given account pool.
func NewRewriter(pool *accounts.Pool) *Rewriter {
	return &Rewriter{pool: pool, cache: map[string]int{}}
}

// addressFieldPaths are top-level tx_json fields whose value is an address.
var addressFieldPaths = []string{
	"Account", "Destination", "Owner", "RegularKey", "Issuer", "Authorize",
	"Unauthorize", "NFTokenMinter", "Amendment",
}

// stripped lists server-assigned fields the rewriter must remove so rippled
// re-fills them on submit.
var stripped = []string{
	"Sequence", "SigningPubKey", "TxnSignature", "Signers", "hash", "Fee",
	"LastLedgerSequence", "TicketSequence",
}

// iouShapedFields are top-level fields whose value may be an object with an issuer.
var iouShapedFields = []string{
	"Amount", "TakerPays", "TakerGets", "LimitAmount", "SendMax", "DeliverMin", "FeeBase",
}

// Rewrite returns a rewritten copy of tx. ok=false and a reason string are
// returned when the rewrite produces an invariant-violating tx (e.g.,
// Account collapsed to same value as Issuer) — caller should skip it.
func (rw *Rewriter) Rewrite(tx map[string]any) (map[string]any, bool, string) {
	out := cloneDeepAny(tx)

	// Strip server-assigned fields.
	for _, f := range stripped {
		delete(out, f)
	}

	// Rewrite top-level address fields.
	for _, f := range addressFieldPaths {
		if v, ok := out[f].(string); ok && v != "" {
			out[f] = rw.mapAddress(v)
		}
	}

	// Rewrite IOU issuer fields.
	for _, f := range iouShapedFields {
		if m, ok := out[f].(map[string]any); ok {
			if iss, ok := m["issuer"].(string); ok && iss != "" {
				m["issuer"] = rw.mapAddress(iss)
			}
		}
	}

	// Invariant: Account must not equal Issuer after rewrite for IOU-shaped
	// fields (TrustSet / OfferCreate / Payment-with-IOU would be rejected).
	if acct, ok := out["Account"].(string); ok {
		for _, f := range iouShapedFields {
			if m, ok := out[f].(map[string]any); ok {
				if iss, ok := m["issuer"].(string); ok && iss == acct {
					return nil, false, "collapsed Account==Issuer after rewrite (pool too small for tx)"
				}
			}
		}
	}

	return out, true, ""
}

// mapAddress returns the pool-account address for mainnetAddr, memoizing so
// the same mainnet address always maps to the same pool account within a
// single rewriter's lifetime. Each distinct mainnet address is assigned the
// next pool slot (round-robin) so different addresses are as spread out as
// possible across the pool, minimising spurious Account==Issuer collisions.
func (rw *Rewriter) mapAddress(mainnetAddr string) string {
	if idx, ok := rw.cache[mainnetAddr]; ok {
		return rw.pool.All()[idx].ClassicAddress
	}
	idx := rw.counter % len(rw.pool.All())
	rw.counter++
	rw.cache[mainnetAddr] = idx
	return rw.pool.All()[idx].ClassicAddress
}

// SecretFor returns the pool-account seed matching Account after rewrite.
func (rw *Rewriter) SecretFor(account string) (string, bool) {
	for _, w := range rw.pool.All() {
		if w.ClassicAddress == account {
			return w.Seed, true
		}
	}
	return "", false
}

// cloneDeepAny recursively copies a map[string]any so mutation of the copy
// doesn't alter the original.
func cloneDeepAny(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		switch t := v.(type) {
		case map[string]any:
			out[k] = cloneDeepAny(t)
		case []any:
			out[k] = cloneArr(t)
		default:
			out[k] = v
		}
	}
	return out
}

func cloneArr(a []any) []any {
	out := make([]any, len(a))
	for i, v := range a {
		switch t := v.(type) {
		case map[string]any:
			out[i] = cloneDeepAny(t)
		case []any:
			out[i] = cloneArr(t)
		default:
			out[i] = v
		}
	}
	return out
}
