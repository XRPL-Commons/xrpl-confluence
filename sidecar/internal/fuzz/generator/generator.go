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
	pool *accounts.Pool
}

// New constructs a Generator over the given pool.
func New(pool *accounts.Pool) *Generator {
	return &Generator{pool: pool}
}
