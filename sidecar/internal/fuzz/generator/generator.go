package generator

import (
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/accounts"
)

// Tx is the fuzzer's transport-neutral transaction representation. Fields
// holds the full tx_json object (TransactionType + all tx-specific fields);
// Secret is kept separate so it's never accidentally serialized on-wire.
type Tx struct {
	Fields map[string]any `json:"-"`
	Secret string         `json:"-"`
}

// TransactionType returns the TransactionType field, for convenience.
func (t *Tx) TransactionType() string {
	if s, ok := t.Fields["TransactionType"].(string); ok {
		return s
	}
	return ""
}

// Generator builds well-formed transactions from an account pool. M1 ships
// three tx types: Payment, TrustSet, OfferCreate.
type Generator struct {
	pool    *accounts.Pool
	mutator *Mutator
}

// New constructs a Generator over the given pool.
func New(pool *accounts.Pool) *Generator {
	return &Generator{pool: pool, mutator: NewMutator()}
}

// Mutator exposes the Generator's mutator so callers can apply mutations
// post-PickTx. Kept separate from PickTx so tests can run zero-mutation.
func (g *Generator) Mutator() *Mutator {
	return g.mutator
}
