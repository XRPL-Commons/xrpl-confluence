// Package oracle compares ledger hashes across multiple XRPL nodes to detect
// consensus divergences.
package oracle

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// Node represents a named XRPL node with an RPC client.
type Node struct {
	Name   string
	Client *rpcclient.Client
}

// Oracle compares ledger hashes across all nodes.
type Oracle struct {
	nodes []Node
}

// New creates a new Oracle with the given nodes.
func New(nodes []Node) *Oracle {
	return &Oracle{nodes: nodes}
}

// LedgerComparison holds the result of comparing a single ledger across nodes.
type LedgerComparison struct {
	Sequence    int           `json:"sequence"`
	Agreed      bool          `json:"agreed"`
	NodeHashes  []NodeHash    `json:"node_hashes"`
	Divergences []Divergence  `json:"divergences,omitempty"`
	Errors      []string      `json:"errors,omitempty"`
}

// NodeHash holds the three root hashes reported by a single node.
type NodeHash struct {
	Name            string `json:"name"`
	LedgerHash      string `json:"ledger_hash"`
	AccountHash     string `json:"account_hash"`
	TransactionHash string `json:"transaction_hash"`
}

// Divergence describes a hash mismatch between two nodes.
type Divergence struct {
	Field string `json:"field"` // "ledger_hash", "account_hash", or "transaction_hash"
	NodeA string `json:"node_a"`
	HashA string `json:"hash_a"`
	NodeB string `json:"node_b"`
	HashB string `json:"hash_b"`
}

// CompareAtSequence queries all nodes for a specific ledger sequence and
// compares their hashes. It waits for all nodes to have validated that
// sequence before comparing.
func (o *Oracle) CompareAtSequence(ctx context.Context, seq int) *LedgerComparison {
	result := &LedgerComparison{Sequence: seq, Agreed: true}

	// Wait for all nodes to have validated this sequence.
	for _, node := range o.nodes {
		if err := o.waitForValidated(ctx, node, seq); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", node.Name, err))
			result.Agreed = false
			return result
		}
	}

	// Query each node for the ledger hashes.
	for _, node := range o.nodes {
		ledger, err := node.Client.Ledger(seq)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", node.Name, err))
			result.Agreed = false
			continue
		}
		result.NodeHashes = append(result.NodeHashes, NodeHash{
			Name:            node.Name,
			LedgerHash:      ledger.LedgerHash,
			AccountHash:     ledger.AccountHash,
			TransactionHash: ledger.TransactionHash,
		})
	}

	// Compare all node hashes against the first node.
	if len(result.NodeHashes) < 2 {
		return result
	}

	ref := result.NodeHashes[0]
	for i := 1; i < len(result.NodeHashes); i++ {
		nh := result.NodeHashes[i]
		if nh.LedgerHash != ref.LedgerHash {
			result.Agreed = false
			result.Divergences = append(result.Divergences, Divergence{
				Field: "ledger_hash",
				NodeA: ref.Name, HashA: ref.LedgerHash,
				NodeB: nh.Name, HashB: nh.LedgerHash,
			})
		}
		if nh.AccountHash != ref.AccountHash {
			result.Agreed = false
			result.Divergences = append(result.Divergences, Divergence{
				Field: "account_hash",
				NodeA: ref.Name, HashA: ref.AccountHash,
				NodeB: nh.Name, HashB: nh.AccountHash,
			})
		}
		if nh.TransactionHash != ref.TransactionHash {
			result.Agreed = false
			result.Divergences = append(result.Divergences, Divergence{
				Field: "transaction_hash",
				NodeA: ref.Name, HashA: ref.TransactionHash,
				NodeB: nh.Name, HashB: nh.TransactionHash,
			})
		}
	}

	return result
}

// WatchAndCompare continuously compares ledgers starting from startSeq.
// It sends results on the returned channel until ctx is cancelled.
func (o *Oracle) WatchAndCompare(ctx context.Context, startSeq int) <-chan *LedgerComparison {
	ch := make(chan *LedgerComparison, 32)

	go func() {
		defer close(ch)
		seq := startSeq

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			result := o.CompareAtSequence(ctx, seq)
			if ctx.Err() != nil {
				return
			}

			select {
			case ch <- result:
			case <-ctx.Done():
				return
			}

			if !result.Agreed {
				log.Printf("DIVERGENCE at seq %d: %v", seq, result.Divergences)
			}

			seq++
		}
	}()

	return ch
}

// waitForValidated polls a node until it has validated the given sequence.
func (o *Oracle) waitForValidated(ctx context.Context, node Node, seq int) error {
	timeout := 120 * time.Second
	deadline := time.Now().Add(timeout)

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for validated seq >= %d", seq)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		info, err := node.Client.ServerInfo()
		if err != nil {
			// Node might not be ready yet, retry.
			time.Sleep(500 * time.Millisecond)
			continue
		}

		if info.Validated.Seq >= seq {
			return nil
		}

		time.Sleep(500 * time.Millisecond)
	}
}
