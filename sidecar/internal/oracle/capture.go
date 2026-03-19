package oracle

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
)

// DivergenceCapture holds all diagnostic data collected when a hash divergence
// is detected between nodes.
type DivergenceCapture struct {
	DetectedAtSeq int                    `json:"detected_at_seq"`
	LastGoodSeq   int                    `json:"last_good_seq"`
	Comparison    *LedgerComparison      `json:"comparison"`
	Transactions  []CapturedTx           `json:"transactions"`
	AccountStates map[string][]NodeState `json:"account_states,omitempty"`
}

// CapturedTx holds a transaction from the divergent ledger.
type CapturedTx struct {
	TxHash string                 `json:"tx_hash"`
	TxJSON map[string]interface{} `json:"tx_json"`
	Meta   map[string]interface{} `json:"meta,omitempty"`
}

// NodeState holds account state as reported by a specific node.
type NodeState struct {
	NodeName string                 `json:"node_name"`
	Account  string                 `json:"account"`
	Data     map[string]interface{} `json:"data"`
	Error    string                 `json:"error,omitempty"`
}

// CaptureDivergence collects diagnostic data when a divergence is detected.
// It queries each node for the transactions in the divergent ledger and the
// account states of all accounts involved in those transactions.
func (o *Oracle) CaptureDivergence(ctx context.Context, lastGoodSeq, badSeq int, comparison *LedgerComparison) *DivergenceCapture {
	capture := &DivergenceCapture{
		DetectedAtSeq: badSeq,
		LastGoodSeq:   lastGoodSeq,
		Comparison:    comparison,
		AccountStates: make(map[string][]NodeState),
	}

	// Query each node for the transactions in the divergent ledger.
	// Use the first node that responds with a valid ledger.
	txs := o.captureTransactions(ctx, badSeq)
	capture.Transactions = txs

	// Extract all accounts involved in the transactions.
	accounts := extractAccounts(txs)

	// Query account state on each node for comparison.
	for _, account := range accounts {
		for _, node := range o.nodes {
			info, err := node.Client.AccountInfo(account)
			if err != nil {
				capture.AccountStates[account] = append(capture.AccountStates[account], NodeState{
					NodeName: node.Name,
					Account:  account,
					Error:    err.Error(),
				})
				continue
			}
			capture.AccountStates[account] = append(capture.AccountStates[account], NodeState{
				NodeName: node.Name,
				Account:  account,
				Data: map[string]interface{}{
					"Balance":  info.Balance,
					"Sequence": info.Sequence,
				},
			})
		}
	}

	return capture
}

// captureTransactions queries each node for the transactions in a specific ledger
// and returns the union of all transactions found.
func (o *Oracle) captureTransactions(ctx context.Context, seq int) []CapturedTx {
	var txs []CapturedTx

	for _, node := range o.nodes {
		raw, err := node.Client.Call("ledger", map[string]interface{}{
			"ledger_index": seq,
			"transactions": true,
			"expand":       true,
		})
		if err != nil {
			log.Printf("  capture: failed to get ledger %d from %s: %v", seq, node.Name, err)
			continue
		}

		var wrapper struct {
			Ledger struct {
				Transactions []json.RawMessage `json:"transactions"`
			} `json:"ledger"`
		}
		if err := json.Unmarshal(raw, &wrapper); err != nil {
			log.Printf("  capture: failed to parse ledger response from %s: %v", node.Name, err)
			continue
		}

		for _, txRaw := range wrapper.Ledger.Transactions {
			var txData map[string]interface{}
			if err := json.Unmarshal(txRaw, &txData); err != nil {
				continue
			}

			hash, _ := txData["hash"].(string)
			// Avoid duplicates if we already captured this tx from another node.
			if !hasTx(txs, hash) {
				ct := CapturedTx{
					TxHash: hash,
					TxJSON: txData,
				}
				if meta, ok := txData["metaData"]; ok {
					if metaMap, ok := meta.(map[string]interface{}); ok {
						ct.Meta = metaMap
					}
				}
				txs = append(txs, ct)
			}
		}

		// One node's data is usually sufficient.
		if len(txs) > 0 {
			break
		}
	}

	return txs
}

func hasTx(txs []CapturedTx, hash string) bool {
	for _, tx := range txs {
		if tx.TxHash == hash {
			return true
		}
	}
	return false
}

// extractAccounts collects all unique account addresses from the captured transactions.
func extractAccounts(txs []CapturedTx) []string {
	seen := make(map[string]bool)
	var accounts []string

	for _, tx := range txs {
		for _, field := range []string{"Account", "Destination", "Owner"} {
			if addr, ok := tx.TxJSON[field].(string); ok && !seen[addr] {
				seen[addr] = true
				accounts = append(accounts, addr)
			}
		}
	}

	return accounts
}

// MarshalJSON returns a JSON representation of the capture for saving to disk.
func (c *DivergenceCapture) MarshalJSON() ([]byte, error) {
	type alias DivergenceCapture
	return json.MarshalIndent((*alias)(c), "", "  ")
}

// ExportFixture converts a DivergenceCapture into a fixture compatible with
// the goXRPL conformance runner (xrpl-fixtures format).
func (c *DivergenceCapture) ExportFixture() map[string]interface{} {
	steps := []interface{}{}

	// Fund step for each account involved.
	accounts := extractAccounts(c.Transactions)
	for _, account := range accounts {
		steps = append(steps, map[string]interface{}{
			"type":    "fund",
			"account": account,
			"amount":  "10000000000",
		})
	}

	// Close ledger to commit funding.
	steps = append(steps, map[string]interface{}{
		"type": "close",
	})

	// Replay each transaction.
	for _, tx := range c.Transactions {
		step := map[string]interface{}{
			"type":    "tx",
			"tx_json": tx.TxJSON,
		}
		if tx.Meta != nil {
			if ter, ok := tx.Meta["TransactionResult"].(string); ok {
				step["expect_ter"] = ter
			}
		}
		steps = append(steps, step)
	}

	// Final close.
	steps = append(steps, map[string]interface{}{
		"type": "close",
	})

	return map[string]interface{}{
		"rippled_version": "consensus-divergence",
		"suite":           "xrpl-confluence.consensus",
		"testcase":        fmt.Sprintf("divergence_at_seq_%d", c.DetectedAtSeq),
		"env": map[string]interface{}{
			"base_fee":           10,
			"reserve_base":       200000000,
			"reserve_increment":  50000000,
		},
		"steps": steps,
	}
}
