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

	// Seed from the live tips, then enrich from the validated-history window so
	// a node that has drifted off the modal tip but still observed this seq is
	// included in the comparison.
	bySeq := s.nodePoller.validatedHashesBySeq()
	hashByNode := make(map[string]string)
	for _, n := range snap.Nodes {
		if n.ValidatedLedger != nil && n.ValidatedLedger.Seq == targetSeq && n.ValidatedLedger.Hash != "" {
			hashByNode[n.Name] = n.ValidatedLedger.Hash
		}
	}
	for node, hash := range bySeq[targetSeq] {
		if _, ok := hashByNode[node]; !ok {
			hashByNode[node] = hash
		}
	}

	diverged := len(distinctHashes(hashByNode)) >= 2

	// A clean modal tip does not mean a clean fleet: a wedge/partition fork
	// freezes the diverged node at an earlier seq while everyone else advances,
	// so the conflict never shows up at the current tip. Scan the history
	// window for the earliest seq where nodes disagree and report THAT as the
	// divergence — this is what keeps a genuine fork from going unreported.
	// An explicit ?at= asks strictly about that seq, so we skip the scan there.
	if !diverged && !useAt {
		if forkSeq, forkHashes, ok := earliestForkedSeq(bySeq); ok {
			targetSeq = forkSeq
			hashByNode = forkHashes
			diverged = true
		}
	}

	writeJSON(w, http.StatusOK, StateDiffResponse{
		Ledger:     targetSeq,
		HashByNode: hashByNode,
		Diverged:   diverged,
		AsOf:       snap.Timestamp,
	})
}

// earliestForkedSeq returns the lowest seq in the history window at which two
// or more nodes report different validated hashes, together with that seq's
// node->hash map. The earliest conflict is the fork point and the most useful
// thing to surface. Returns ok=false when no seq is forked.
func earliestForkedSeq(bySeq map[int]map[string]string) (int, map[string]string, bool) {
	forkSeq := 0
	var forkHashes map[string]string
	found := false
	for seq, byNode := range bySeq {
		if len(byNode) < 2 || len(distinctHashes(byNode)) < 2 {
			continue
		}
		if !found || seq < forkSeq {
			forkSeq = seq
			forkHashes = byNode
			found = true
		}
	}
	return forkSeq, forkHashes, found
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
