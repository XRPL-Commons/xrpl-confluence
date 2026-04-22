package oracle

import (
	"context"
	"fmt"
)

// TxResultComparison holds per-node result codes for a single transaction
// hash and flags whether all nodes agreed.
type TxResultComparison struct {
	TxHash      string         `json:"tx_hash"`
	Agreed      bool           `json:"agreed"`
	NodeResults []NodeTxResult `json:"node_results"`
	Errors      []string       `json:"errors,omitempty"`
}

// NodeTxResult is the per-node TransactionResult for one tx.
type NodeTxResult struct {
	Name   string `json:"name"`
	Result string `json:"result"`
}

// CompareTxResult looks up `hash` on every node and compares the
// TransactionResult codes. If any node reports a different code, Agreed is
// false and the full per-node breakdown is returned. Nodes that fail to
// respond contribute an entry to Errors; the comparison is only "agreed"
// when every node returned the same code successfully.
func (o *Oracle) CompareTxResult(ctx context.Context, hash string) *TxResultComparison {
	cmp := &TxResultComparison{TxHash: hash, Agreed: true}

	for _, n := range o.nodes {
		if ctx.Err() != nil {
			cmp.Agreed = false
			cmp.Errors = append(cmp.Errors, "ctx cancelled")
			return cmp
		}
		res, err := n.Client.Tx(hash)
		if err != nil {
			cmp.Agreed = false
			cmp.Errors = append(cmp.Errors, fmt.Sprintf("%s: %v", n.Name, err))
			continue
		}
		cmp.NodeResults = append(cmp.NodeResults, NodeTxResult{
			Name: n.Name, Result: res.TransactionResult,
		})
	}
	if len(cmp.NodeResults) < 2 {
		return cmp
	}
	ref := cmp.NodeResults[0].Result
	for _, nr := range cmp.NodeResults[1:] {
		if nr.Result != ref {
			cmp.Agreed = false
			break
		}
	}
	return cmp
}
