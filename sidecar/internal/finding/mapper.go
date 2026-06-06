package finding

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
)

func MapDivergence(d corpus.Divergence) (api.Finding, error) {
	kind := mapKind(d.Kind)

	var detail json.RawMessage
	if len(d.Details) > 0 {
		b, err := json.Marshal(d.Details)
		if err != nil {
			return api.Finding{}, fmt.Errorf("marshal details: %w", err)
		}
		detail = b
	}

	return api.Finding{
		ID:       NewFindingID(),
		Kind:     kind,
		Severity: api.SeverityError,
		OpenedAt: d.RecordedAt,
		Summary:  d.Description,
		Detail:   detail,
	}, nil
}

func mapKind(k string) string {
	switch k {
	case "state_hash", "tx_result", "metadata":
		return api.KindStateDivergence
	case "consensus_stall":
		return api.KindConsensusStall
	case "peer_drop":
		return api.KindPeerDrop
	case "invariant", "chaos":
		return api.KindChaosViolation
	case "crash":
		return api.KindNodeCrash
	case "setup_failure":
		return api.KindFuzzFailure
	default:
		// Unknown kinds must NOT default to state_divergence: a stall or a
		// harness hiccup masquerading as a ledger fork is exactly the false
		// positive that makes the dashboard cry "desync" when every node
		// agrees. Bucket the unknown as a generic fuzz failure instead.
		log.Printf("finding: unknown corpus kind %q, defaulting to %s", k, api.KindFuzzFailure)
		return api.KindFuzzFailure
	}
}
