package oracle

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

// TxMetadataComparison is the cross-node diff for a single tx's AffectedNodes.
// Agreed is true iff every node returned byte-equal AffectedNodes JSON.
type TxMetadataComparison struct {
	TxHash   string         `json:"tx_hash"`
	Agreed   bool           `json:"agreed"`
	NodeMeta []NodeMetaBlob `json:"node_meta"`
	Errors   []string       `json:"errors,omitempty"`
}

// NodeMetaBlob is the AffectedNodes blob observed on one node.
type NodeMetaBlob struct {
	Name          string          `json:"name"`
	AffectedNodes json.RawMessage `json:"affected_nodes"`
}

// CompareTxMetadata fetches the tx on every node and compares the raw
// AffectedNodes JSON byte-for-byte. rippled emits AffectedNodes in a canonical
// order, so byte-equality is a correct proxy for structural equality.
//
// Primarily diagnostic: if layer 1 (ledger-hash) agrees, layer 3 must also
// agree by construction. When layer 1 reports a divergence, layer 3 tells
// you *which* state objects each node thinks the tx affected.
func (o *Oracle) CompareTxMetadata(ctx context.Context, hash string) *TxMetadataComparison {
	cmp := &TxMetadataComparison{TxHash: hash, Agreed: true}

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
		cmp.NodeMeta = append(cmp.NodeMeta, NodeMetaBlob{
			Name:          n.Name,
			AffectedNodes: res.AffectedNodes,
		})
	}
	if len(cmp.NodeMeta) < 2 {
		return cmp
	}
	ref := cmp.NodeMeta[0].AffectedNodes
	for _, nm := range cmp.NodeMeta[1:] {
		if !bytes.Equal(nm.AffectedNodes, ref) {
			cmp.Agreed = false
			break
		}
	}
	return cmp
}
