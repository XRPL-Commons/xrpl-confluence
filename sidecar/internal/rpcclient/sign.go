// Local signing helper for the rpcclient.
//
// SignLocal autofills Sequence / Fee / LastLedgerSequence / SigningPubKey on
// a tx map then hands it to xrpl-go's wallet.Sign to produce a hex-encoded
// tx_blob. The blob can then be submitted via SubmitTxBlob.

package rpcclient

import (
	"encoding/json"
	"fmt"

	"github.com/Peersyst/xrpl-go/xrpl/wallet"
)

// SignLocal autofills the standard tx fields rippled would otherwise fill
// during sign_and_submit, then signs the tx with the given XRPL secret.
// Returns the hex-encoded tx_blob ready for SubmitTxBlob.
func (c *Client) SignLocal(secret string, tx map[string]any) (string, error) {
	w, err := wallet.FromSeed(secret, "")
	if err != nil {
		return "", fmt.Errorf("wallet from seed: %w", err)
	}

	if _, ok := tx["Account"]; !ok {
		tx["Account"] = w.ClassicAddress.String()
	}

	if _, ok := tx["Sequence"]; !ok {
		account, ok2 := tx["Account"].(string)
		if !ok2 {
			return "", fmt.Errorf("Account field is not a string: %T", tx["Account"])
		}
		info, err := c.AccountInfo(account)
		if err != nil {
			return "", fmt.Errorf("account_info: %w", err)
		}
		tx["Sequence"] = info.Sequence
	}

	if _, ok := tx["Fee"]; !ok {
		tx["Fee"] = "10"
	}

	if _, ok := tx["LastLedgerSequence"]; !ok {
		raw, err := c.Call("ledger_current", map[string]any{})
		if err != nil {
			return "", fmt.Errorf("ledger_current: %w", err)
		}
		var lc struct {
			LedgerCurrentIndex int `json:"ledger_current_index"`
		}
		if err := json.Unmarshal(raw, &lc); err != nil {
			return "", fmt.Errorf("parse ledger_current: %w", err)
		}
		tx["LastLedgerSequence"] = lc.LedgerCurrentIndex + 20
	}

	blob, _, err := w.Sign(tx)
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}
	return blob, nil
}
