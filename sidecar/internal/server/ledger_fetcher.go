package server

import (
	"context"
	"fmt"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/finding"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// rpcLedgerFetcher implements finding.LedgerFetcher by calling LedgerWithTxs
// against the per-node RPC endpoint discovered from the supplied node configs.
// It is intentionally stateless aside from the {name -> RPC URL} map; each
// call constructs a fresh rpcclient so the underlying http.Client never holds
// long-lived state that could leak under enclave churn.
type rpcLedgerFetcher struct {
	rpcByNode map[string]string
}

// NewRPCLedgerFetcher builds a fetcher from the same NodeConfig slice the
// NodePoller is using, so every node the poller observes can also be diffed.
func NewRPCLedgerFetcher(cfgs []NodeConfig) *rpcLedgerFetcher {
	m := make(map[string]string, len(cfgs))
	for _, c := range cfgs {
		m[c.Name] = c.RPC
	}
	return &rpcLedgerFetcher{rpcByNode: m}
}

func (f *rpcLedgerFetcher) FetchLedger(_ context.Context, node string, seq int) (*finding.LedgerSnapshot, error) {
	rpc, ok := f.rpcByNode[node]
	if !ok {
		return nil, fmt.Errorf("unknown node %q", node)
	}
	res, err := rpcclient.New(rpc).LedgerWithTxs(seq)
	if err != nil {
		return nil, err
	}
	out := &finding.LedgerSnapshot{
		Node:            node,
		Seq:             res.Seq,
		LedgerHash:      res.LedgerHash,
		AccountHash:     res.AccountHash,
		TransactionHash: res.TransactionHash,
	}
	for _, tx := range res.Transactions {
		out.Transactions = append(out.Transactions, finding.LedgerTxSnapshot{
			Hash:            tx.Hash,
			TransactionType: tx.TransactionType,
			Account:         tx.Account,
			Meta:            tx.Meta,
		})
	}
	return out, nil
}
