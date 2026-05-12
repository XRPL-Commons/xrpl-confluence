package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
)

// StateDiffResponse is the body for GET /v1/state/diff.
type StateDiffResponse struct {
	Ledger     int               `json:"ledger"`
	HashByNode map[string]string `json:"hash_by_node"`
	Diverged   bool              `json:"diverged"`
	AsOf       int64             `json:"as_of"` // unix millis
}

func (s *Server) stateDiff(w http.ResponseWriter, r *http.Request) {
	rawAt := r.URL.Query().Get("at")

	var atSeq int
	useAt := false
	if rawAt != "" {
		n, err := strconv.Atoi(rawAt)
		if err != nil || n <= 0 {
			writeJSON(w, http.StatusBadRequest, api.ErrorResponse{
				Error: api.Error{Code: api.ErrCodeBadRequest, Message: "at must be a positive integer"},
			})
			return
		}
		atSeq = n
		useAt = true
	}

	if s.nodePoller == nil {
		writeJSON(w, http.StatusOK, StateDiffResponse{
			Ledger:     0,
			HashByNode: map[string]string{},
			Diverged:   false,
			AsOf:       time.Now().UnixMilli(),
		})
		return
	}

	snap := s.nodePoller.Snapshot()

	var targetSeq int
	if useAt {
		targetSeq = atSeq
	} else {
		targetSeq = pickSeq(snap.Nodes)
	}

	hashByNode := make(map[string]string)
	for _, n := range snap.Nodes {
		if n.ValidatedLedger != nil && n.ValidatedLedger.Seq == targetSeq {
			hashByNode[n.Name] = n.ValidatedLedger.Hash
		}
	}

	distinct := distinctHashes(hashByNode)

	writeJSON(w, http.StatusOK, StateDiffResponse{
		Ledger:     targetSeq,
		HashByNode: hashByNode,
		Diverged:   len(distinct) >= 2,
		AsOf:       snap.Timestamp,
	})
}

// pickSeq returns the ledger seq agreed upon by the most nodes.
// Ties are broken by the highest seq.
func pickSeq(nodes []Node) int {
	counts := make(map[int]int)
	for _, n := range nodes {
		if n.ValidatedLedger != nil && n.ValidatedLedger.Seq > 0 {
			counts[n.ValidatedLedger.Seq]++
		}
	}
	best := 0
	bestCount := 0
	for seq, count := range counts {
		if count > bestCount || (count == bestCount && seq > best) {
			best = seq
			bestCount = count
		}
	}
	return best
}

func distinctHashes(m map[string]string) map[string]struct{} {
	out := make(map[string]struct{}, len(m))
	for _, h := range m {
		out[h] = struct{}{}
	}
	return out
}
