package corpus

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// DivergenceSignature is the comparable shape used by the shrinker to decide
// whether a freshly-observed Divergence is the "same bug" as the original.
//
// Matching policy:
//
//	Kind       always required (exact match)
//	TxType     for kind in {tx_result, metadata} — exact match
//	Field      for kind == state_hash — exact match against
//	           comparison.divergences[0].field
//	Invariant  for kind == invariant — exact match against details.invariant
//
// Empty subfields on the signature are treated as wildcards.
type DivergenceSignature struct {
	Kind      string `json:"kind"`
	TxType    string `json:"tx_type,omitempty"`
	Field     string `json:"field,omitempty"`
	Invariant string `json:"invariant,omitempty"`
}

// LoadDivergenceSignature reads a divergence JSON file (as produced by
// Recorder.RecordDivergence) and derives its signature.
func LoadDivergenceSignature(path string) (DivergenceSignature, error) {
	var sig DivergenceSignature
	data, err := os.ReadFile(path)
	if err != nil {
		return sig, fmt.Errorf("read divergence: %w", err)
	}
	var d Divergence
	if err := json.Unmarshal(data, &d); err != nil {
		return sig, fmt.Errorf("parse divergence: %w", err)
	}
	if d.Kind == "" {
		return sig, fmt.Errorf("divergence has no kind")
	}
	sig.Kind = d.Kind
	switch d.Kind {
	case "tx_result", "metadata":
		if v, ok := d.Details["tx_type"].(string); ok {
			sig.TxType = v
		}
	case "state_hash":
		sig.Field = stateHashField(d.Details)
	case "invariant":
		if v, ok := d.Details["invariant"].(string); ok {
			sig.Invariant = v
		}
	}
	return sig, nil
}

// Matches reports whether d is the "same bug" as the signature.
func (s DivergenceSignature) Matches(d *Divergence) bool {
	if d == nil || d.Kind != s.Kind {
		return false
	}
	switch s.Kind {
	case "tx_result", "metadata":
		if s.TxType == "" {
			return true
		}
		v, _ := d.Details["tx_type"].(string)
		return v == s.TxType
	case "state_hash":
		if s.Field == "" {
			return true
		}
		return stateHashField(d.Details) == s.Field
	case "invariant":
		if s.Invariant == "" {
			return true
		}
		v, _ := d.Details["invariant"].(string)
		return v == s.Invariant
	}
	return true
}

// Signature derives a signature directly from an in-memory Divergence —
// the same logic as LoadDivergenceSignature but without disk IO. Returns
// the zero value for nil input.
func Signature(d *Divergence) DivergenceSignature {
	var s DivergenceSignature
	if d == nil {
		return s
	}
	s.Kind = d.Kind
	switch d.Kind {
	case "tx_result", "metadata":
		if v, ok := d.Details["tx_type"].(string); ok {
			s.TxType = v
		}
	case "state_hash":
		s.Field = stateHashField(d.Details)
	case "invariant":
		if v, ok := d.Details["invariant"].(string); ok {
			s.Invariant = v
		}
	}
	return s
}

// Key returns a filesystem-safe stable identifier for this signature.
// Used as the per-signature index directory name and as the dedup key
// for the alert webhook.
func (s DivergenceSignature) Key() string {
	parts := []string{s.Kind}
	for _, p := range []string{s.TxType, s.Field, s.Invariant} {
		if p != "" {
			parts = append(parts, sanitize(p))
		}
	}
	return strings.Join(parts, "_")
}

func sanitize(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_', c == '.':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}

func stateHashField(details map[string]any) string {
	cmp, ok := details["comparison"].(map[string]any)
	if !ok {
		return ""
	}
	divs, ok := cmp["divergences"].([]any)
	if !ok || len(divs) == 0 {
		return ""
	}
	d0, ok := divs[0].(map[string]any)
	if !ok {
		return ""
	}
	v, _ := d0["field"].(string)
	return v
}
