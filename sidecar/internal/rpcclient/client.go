// Package rpcclient provides a thin HTTP JSON-RPC client for XRPL nodes.
package rpcclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is an XRPL JSON-RPC client.
type Client struct {
	endpoint string
	http     *http.Client
}

// New creates a new RPC client for the given endpoint (e.g. "http://rippled-0:5005").
func New(endpoint string) *Client {
	return &Client{
		endpoint: endpoint,
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// rpcRequest is the JSON-RPC request envelope.
type rpcRequest struct {
	Method string        `json:"method"`
	Params []interface{} `json:"params"`
}

// rpcResponse is the JSON-RPC response envelope.
type rpcResponse struct {
	Result json.RawMessage `json:"result"`
}

// Call invokes an RPC method and returns the raw result.
func (c *Client) Call(method string, params interface{}) (json.RawMessage, error) {
	p := []interface{}{}
	if params != nil {
		p = []interface{}{params}
	} else {
		p = []interface{}{map[string]interface{}{}}
	}

	req := rpcRequest{Method: method, Params: p}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.http.Post(c.endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("http post to %s: %w", c.endpoint, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var rpcResp rpcResponse
	if err := json.Unmarshal(data, &rpcResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w (body: %s)", err, string(data))
	}

	return rpcResp.Result, nil
}

// ServerInfoResult holds relevant fields from server_info.
type ServerInfoResult struct {
	ServerState string `json:"server_state"`
	Peers       int    `json:"peers"`
	Validated   struct {
		Seq  int    `json:"seq"`
		Hash string `json:"hash"`
	}
}

// ServerInfo calls server_info and returns parsed results.
func (c *Client) ServerInfo() (*ServerInfoResult, error) {
	raw, err := c.Call("server_info", nil)
	if err != nil {
		return nil, err
	}

	var wrapper struct {
		Info struct {
			ServerState     string `json:"server_state"`
			Peers           int    `json:"peers"`
			ValidatedLedger struct {
				Seq  int    `json:"seq"`
				Hash string `json:"hash"`
			} `json:"validated_ledger"`
		} `json:"info"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("parse server_info: %w", err)
	}

	return &ServerInfoResult{
		ServerState: wrapper.Info.ServerState,
		Peers:       wrapper.Info.Peers,
		Validated: struct {
			Seq  int    `json:"seq"`
			Hash string `json:"hash"`
		}{
			Seq:  wrapper.Info.ValidatedLedger.Seq,
			Hash: wrapper.Info.ValidatedLedger.Hash,
		},
	}, nil
}

// LedgerResult holds the three root hashes for a ledger.
type LedgerResult struct {
	Seq             int    `json:"seq"`
	LedgerHash      string `json:"ledger_hash"`
	AccountHash     string `json:"account_hash"`
	TransactionHash string `json:"transaction_hash"`
}

// Ledger fetches a specific ledger by sequence number.
func (c *Client) Ledger(seq int) (*LedgerResult, error) {
	raw, err := c.Call("ledger", map[string]interface{}{
		"ledger_index": seq,
	})
	if err != nil {
		return nil, err
	}

	var wrapper struct {
		Ledger struct {
			LedgerIndex     json.RawMessage `json:"ledger_index"`
			LedgerHash      string          `json:"ledger_hash"`
			AccountHash     string          `json:"account_hash"`
			TransactionHash string          `json:"transaction_hash"`
		} `json:"ledger"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("parse ledger: %w", err)
	}

	if wrapper.Status == "error" {
		return nil, fmt.Errorf("ledger %d not found", seq)
	}

	return &LedgerResult{
		Seq:             seq,
		LedgerHash:      wrapper.Ledger.LedgerHash,
		AccountHash:     wrapper.Ledger.AccountHash,
		TransactionHash: wrapper.Ledger.TransactionHash,
	}, nil
}

// SubmitResult holds the result of a transaction submission.
type SubmitResult struct {
	EngineResult        string
	EngineResultCode    int
	EngineResultMessage string
	TxHash              string
	Sequence            uint32
	Status              string
}

// feeMultMax is passed to every sign-and-submit call so that rippled does not
// reject transactions when the network load_factor temporarily elevates the
// reference fee above the default 10× cushion. 1000× (= 10 000 drops at a 10-
// drop base fee) is deliberately generous for a test network.
const feeMultMax = 1000

// SubmitPayment submits a signed Payment using sign-and-submit.
func (c *Client) SubmitPayment(secret, account, destination, amount string) (*SubmitResult, error) {
	raw, err := c.Call("submit", map[string]interface{}{
		"secret":       secret,
		"fee_mult_max": feeMultMax,
		"tx_json": map[string]interface{}{
			"TransactionType": "Payment",
			"Account":         account,
			"Destination":     destination,
			"Amount":          amount,
		},
	})
	if err != nil {
		return nil, err
	}
	return parseSubmitResult(raw)
}

// SubmitTrustSet submits a TrustSet transaction using sign-and-submit.
func (c *Client) SubmitTrustSet(secret, account, currency, issuer, limit string) (*SubmitResult, error) {
	raw, err := c.Call("submit", map[string]interface{}{
		"secret":       secret,
		"fee_mult_max": feeMultMax,
		"tx_json": map[string]interface{}{
			"TransactionType": "TrustSet",
			"Account":         account,
			"LimitAmount": map[string]interface{}{
				"currency": currency,
				"issuer":   issuer,
				"value":    limit,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return parseSubmitResult(raw)
}

// SubmitOfferCreate submits an OfferCreate transaction using sign-and-submit.
func (c *Client) SubmitOfferCreate(secret, account string, takerPays, takerGets interface{}) (*SubmitResult, error) {
	raw, err := c.Call("submit", map[string]interface{}{
		"secret":       secret,
		"fee_mult_max": feeMultMax,
		"tx_json": map[string]interface{}{
			"TransactionType": "OfferCreate",
			"Account":         account,
			"TakerPays":       takerPays,
			"TakerGets":       takerGets,
		},
	})
	if err != nil {
		return nil, err
	}
	return parseSubmitResult(raw)
}

// SubmitAccountSet submits an AccountSet transaction using sign-and-submit.
func (c *Client) SubmitAccountSet(secret, account string, setFlag uint32) (*SubmitResult, error) {
	raw, err := c.Call("submit", map[string]interface{}{
		"secret":       secret,
		"fee_mult_max": feeMultMax,
		"tx_json": map[string]interface{}{
			"TransactionType": "AccountSet",
			"Account":         account,
			"SetFlag":         setFlag,
		},
	})
	if err != nil {
		return nil, err
	}
	return parseSubmitResult(raw)
}

// WalletProposeResult holds the result of wallet_propose.
type WalletProposeResult struct {
	AccountID  string `json:"account_id"`
	MasterSeed string `json:"master_seed"`
	PublicKey  string `json:"public_key"`
}

// WalletPropose generates a new wallet keypair via the node's admin RPC.
func (c *Client) WalletPropose() (*WalletProposeResult, error) {
	raw, err := c.Call("wallet_propose", nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		AccountID  string `json:"account_id"`
		MasterSeed string `json:"master_seed"`
		PublicKey  string `json:"public_key"`
		Status     string `json:"status"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse wallet_propose: %w", err)
	}

	return &WalletProposeResult{
		AccountID:  result.AccountID,
		MasterSeed: result.MasterSeed,
		PublicKey:  result.PublicKey,
	}, nil
}

// AccountInfoResult holds relevant account_info fields.
type AccountInfoResult struct {
	Account  string `json:"account"`
	Balance  string `json:"balance"`
	Sequence int    `json:"sequence"`
}

// AccountInfo fetches account info.
func (c *Client) AccountInfo(account string) (*AccountInfoResult, error) {
	raw, err := c.Call("account_info", map[string]interface{}{
		"account": account,
	})
	if err != nil {
		return nil, err
	}

	var wrapper struct {
		AccountData struct {
			Account  string `json:"Account"`
			Balance  string `json:"Balance"`
			Sequence int    `json:"Sequence"`
		} `json:"account_data"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("parse account_info: %w", err)
	}

	if wrapper.Status == "error" {
		return nil, fmt.Errorf("account %s not found", account)
	}

	return &AccountInfoResult{
		Account:  wrapper.AccountData.Account,
		Balance:  wrapper.AccountData.Balance,
		Sequence: wrapper.AccountData.Sequence,
	}, nil
}

// TxResult holds the relevant fields from a `tx` RPC response.
type TxResult struct {
	TransactionResult string          `json:"transaction_result"`
	Validated         bool            `json:"validated"`
	AffectedNodes     json.RawMessage `json:"affected_nodes,omitempty"`
}

// Tx looks up a transaction by hash and returns its result code and metadata.
func (c *Client) Tx(hash string) (*TxResult, error) {
	raw, err := c.Call("tx", map[string]any{"transaction": hash})
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Meta struct {
			TransactionResult string          `json:"TransactionResult"`
			AffectedNodes     json.RawMessage `json:"AffectedNodes"`
		} `json:"meta"`
		Validated bool `json:"validated"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("parse tx: %w", err)
	}
	return &TxResult{
		TransactionResult: wrapper.Meta.TransactionResult,
		Validated:         wrapper.Validated,
		AffectedNodes:     wrapper.Meta.AffectedNodes,
	}, nil
}

// SubmitTxJSON submits an arbitrary tx_json object under the given secret.
// Generic path used by fuzzer tx types that don't map to the per-type
// helpers. The tx_json must include TransactionType and Account.
func (c *Client) SubmitTxJSON(secret string, txJSON map[string]any) (*SubmitResult, error) {
	raw, err := c.Call("submit", map[string]interface{}{
		"secret":       secret,
		"fee_mult_max": feeMultMax,
		"tx_json":      txJSON,
	})
	if err != nil {
		return nil, err
	}
	return parseSubmitResult(raw)
}

// SubmitPaymentIOU submits a Payment whose Amount is an IOU (currency+issuer+value)
// rather than a drops string. Useful for setup paths that need to fund trust lines.
func (c *Client) SubmitPaymentIOU(secret, account, destination string, amount map[string]any) (*SubmitResult, error) {
	raw, err := c.Call("submit", map[string]interface{}{
		"secret":       secret,
		"fee_mult_max": feeMultMax,
		"tx_json": map[string]interface{}{
			"TransactionType": "Payment",
			"Account":         account,
			"Destination":     destination,
			"Amount":          amount,
		},
	})
	if err != nil {
		return nil, err
	}
	return parseSubmitResult(raw)
}

// SubmitTxBlob submits a pre-signed transaction blob via the standard
// `submit` RPC. The blob is the hex-encoded XRPL binary form returned by
// xrpl-go's wallet.Sign.
func (c *Client) SubmitTxBlob(blob string) (*SubmitResult, error) {
	raw, err := c.Call("submit", map[string]any{
		"tx_blob": blob,
	})
	if err != nil {
		return nil, err
	}
	return parseSubmitResult(raw)
}

func parseSubmitResult(raw json.RawMessage) (*SubmitResult, error) {
	var result struct {
		EngineResult        string `json:"engine_result"`
		EngineResultCode    int    `json:"engine_result_code"`
		EngineResultMessage string `json:"engine_result_message"`
		TxJSON              struct {
			Hash     string `json:"hash"`
			Sequence uint32 `json:"Sequence"`
		} `json:"tx_json"`
		Status       string `json:"status"`
		Error        string `json:"error"`
		ErrorCode    int    `json:"error_code"`
		ErrorMessage string `json:"error_message"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse submit result: %w", err)
	}

	// When rippled encounters an error before tx processing (e.g. noCurrent,
	// tooBusy, notReady) it returns status="error" with no engine_result.
	// Surface that as an explicit error so callers don't silently see empty strings.
	if result.Status == "error" && result.EngineResult == "" {
		return nil, fmt.Errorf("rpc error %s (%d): %s", result.Error, result.ErrorCode, result.ErrorMessage)
	}

	return &SubmitResult{
		EngineResult:        result.EngineResult,
		EngineResultCode:    result.EngineResultCode,
		EngineResultMessage: result.EngineResultMessage,
		TxHash:              result.TxJSON.Hash,
		Sequence:            result.TxJSON.Sequence,
		Status:              result.Status,
	}, nil
}
