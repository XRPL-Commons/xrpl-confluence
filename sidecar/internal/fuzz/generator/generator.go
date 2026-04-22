package generator

import (
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/accounts"
)

// Tx is the fuzzer's transport-neutral transaction representation. We deliberately
// don't expose xrpl-go's rich tx types at the generator boundary — the runner
// serializes via xrpl-go's json codec and submits through rpcclient.
//
// The xrpl-go types are used to build and validate these structures inside
// the generator (see payment.go et al.).
type Tx struct {
	TransactionType string         `json:"TransactionType"`
	Account         string         `json:"Account"`
	Destination     string         `json:"Destination,omitempty"`
	Amount          any            `json:"Amount,omitempty"`
	LimitAmount     map[string]any `json:"LimitAmount,omitempty"`
	TakerPays       any            `json:"TakerPays,omitempty"`
	TakerGets       any            `json:"TakerGets,omitempty"`

	// Secret is intentionally exported so the runner can sign-and-submit
	// without cracking open the struct; it's NEVER logged or sent into the
	// tx_json on the wire. The runner passes it to the `secret` RPC param.
	Secret string `json:"-"`
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
