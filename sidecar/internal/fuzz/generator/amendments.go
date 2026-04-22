// Package generator produces transactions for the fuzzer via xrpl-go and
// picks candidate transaction types based on the live amendment set.
package generator

import (
	"encoding/json"
	"fmt"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

type featureEntry struct {
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	Supported bool   `json:"supported"`
}

// DiscoverEnabledAmendments calls the node's `feature` admin RPC and returns
// the names of every currently-enabled amendment. The generator uses this set
// at startup to filter its candidate tx types.
func DiscoverEnabledAmendments(client *rpcclient.Client) ([]string, error) {
	raw, err := client.Call("feature", nil)
	if err != nil {
		return nil, fmt.Errorf("feature RPC: %w", err)
	}
	var wrapper struct {
		Features map[string]featureEntry `json:"features"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("parse feature response: %w", err)
	}
	out := make([]string, 0, len(wrapper.Features))
	for _, f := range wrapper.Features {
		if f.Enabled {
			out = append(out, f.Name)
		}
	}
	return out, nil
}
