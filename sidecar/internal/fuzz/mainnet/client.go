// Package mainnet fetches historical ledger data from a public rippled RPC
// endpoint for replay against the confluence topology.
package mainnet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Client is a JSON-RPC client against a public rippled endpoint (e.g.
// https://s1.ripple.com:51234). Read-only RPCs only; no admin.
type Client struct {
	endpoint string
	http     *http.Client
}

// NewClient constructs a client for the given endpoint URL.
func NewClient(endpoint string) *Client {
	return &Client{
		endpoint: endpoint,
		http:     &http.Client{Timeout: 30 * time.Second},
	}
}

type rpcReq struct {
	Method string        `json:"method"`
	Params []interface{} `json:"params"`
}

type rpcResp struct {
	Result json.RawMessage `json:"result"`
}

func (c *Client) call(method string, params map[string]any) (json.RawMessage, error) {
	if params == nil {
		params = map[string]any{}
	}
	req := rpcReq{Method: method, Params: []any{params}}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Post(c.endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("mainnet rpc %s: %w", method, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var wrapper rpcResp
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, fmt.Errorf("parse %s: %w (body: %s)", method, err, string(data))
	}
	return wrapper.Result, nil
}

// LedgerTransactions fetches the full tx list for a given ledger index.
func (c *Client) LedgerTransactions(seq int) ([]map[string]any, error) {
	raw, err := c.call("ledger", map[string]any{
		"ledger_index": seq,
		"transactions": true,
		"expand":       true,
	})
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Ledger struct {
			Transactions []map[string]any `json:"transactions"`
		} `json:"ledger"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("parse ledger %d: %w", seq, err)
	}
	return wrapper.Ledger.Transactions, nil
}

// CurrentValidatedSeq returns the endpoint's last validated ledger sequence.
func (c *Client) CurrentValidatedSeq() (int, error) {
	raw, err := c.call("server_info", nil)
	if err != nil {
		return 0, err
	}
	var wrapper struct {
		Info struct {
			ValidatedLedger struct {
				Seq json.Number `json:"seq"`
			} `json:"validated_ledger"`
		} `json:"info"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return 0, fmt.Errorf("parse server_info: %w", err)
	}
	s, err := strconv.Atoi(string(wrapper.Info.ValidatedLedger.Seq))
	if err != nil {
		return 0, fmt.Errorf("bad seq %q: %w", wrapper.Info.ValidatedLedger.Seq, err)
	}
	return s, nil
}
