package forkdebug

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// StallSample is one snapshot of a node's tip-of-chain state.
type StallSample struct {
	Node          string    `json:"node"`
	At            time.Time `json:"at"`
	ValidatedSeq  int       `json:"validated_seq"`
	ValidatedHash string    `json:"validated_hash"`
	ServerState   string    `json:"server_state"`
	Err           string    `json:"error,omitempty"`
}

// StallResult is the verdict after watching all nodes for the
// configured window.
type StallResult struct {
	WindowSeconds int           `json:"window_seconds"`
	PollInterval  time.Duration `json:"poll_interval"`
	First         []StallSample `json:"first_sample"`
	Last          []StallSample `json:"last_sample"`
	// Stalled is true when EVERY node's validated_seq stayed at the
	// same value across the whole window AND at least one node was
	// in a state that's supposed to be making forward progress
	// (proposing/validating/full). This avoids false positives from
	// freshly-started networks still in syncing/connected.
	Stalled bool `json:"stalled"`
	// PerNodeAdvance is the seq delta (last - first) keyed by node
	// name. Negative deltas (regressions) are surfaced as-is.
	PerNodeAdvance map[string]int `json:"per_node_advance"`
}

// StallDetector polls every node for server_info on a fixed
// interval and reports whether validated_seq advanced across the
// observation window.
type StallDetector struct {
	nodes []scannerNode
}

// NewStallDetector builds a detector over the given nodes.
func NewStallDetector(nodes []Node) (*StallDetector, error) {
	if len(nodes) < 1 {
		return nil, errors.New("stalled needs at least 1 node")
	}
	out := make([]scannerNode, 0, len(nodes))
	for _, n := range nodes {
		if n.Name == "" || n.URL == "" {
			return nil, fmt.Errorf("node missing name or URL: %+v", n)
		}
		out = append(out, scannerNode{Name: n.Name, Client: rpcclient.New(n.URL)})
	}
	return &StallDetector{nodes: out}, nil
}

// Watch samples every node now, sleeps `interval`, samples again
// until the window elapses, and reports the per-node advance.
//
// Honors ctx cancellation between sleeps so a SIGINT mid-watch
// returns a partial result instead of hanging until the window
// completes.
func (d *StallDetector) Watch(ctx context.Context, window time.Duration, interval time.Duration) *StallResult {
	if window <= 0 {
		window = 30 * time.Second
	}
	if interval <= 0 {
		interval = 3 * time.Second
	}

	res := &StallResult{
		WindowSeconds:  int(window / time.Second),
		PollInterval:   interval,
		PerNodeAdvance: make(map[string]int, len(d.nodes)),
	}
	res.First = d.sample()
	deadline := time.Now().Add(window)

	for {
		if time.Now().After(deadline) || ctx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
		case <-time.After(interval):
		}
	}
	res.Last = d.sample()

	// Index first/last samples by node name for delta compute.
	firstByName := make(map[string]int, len(res.First))
	for _, s := range res.First {
		firstByName[s.Node] = s.ValidatedSeq
	}

	allSame := true
	anyAdvanced := false
	anyForwardMode := false

	for _, last := range res.Last {
		first, ok := firstByName[last.Node]
		if !ok {
			continue
		}
		delta := last.ValidatedSeq - first
		res.PerNodeAdvance[last.Node] = delta
		if delta != 0 {
			allSame = false
			if delta > 0 {
				anyAdvanced = true
			}
		}
		switch last.ServerState {
		case "proposing", "validating", "full":
			anyForwardMode = true
		}
	}

	// Stall verdict: nothing advanced AND at least one node is in a
	// state that's supposed to be making progress. Skips noisy
	// "stalled" reports while nodes are still in syncing/connected.
	res.Stalled = allSame && !anyAdvanced && anyForwardMode
	return res
}

// sample takes one synchronous snapshot of every node.
func (d *StallDetector) sample() []StallSample {
	out := make([]StallSample, 0, len(d.nodes))
	now := time.Now()
	for _, n := range d.nodes {
		s := StallSample{Node: n.Name, At: now}
		info, err := n.Client.ServerInfo()
		if err != nil {
			s.Err = err.Error()
			out = append(out, s)
			continue
		}
		s.ValidatedSeq = info.Validated.Seq
		s.ValidatedHash = info.Validated.Hash
		s.ServerState = info.ServerState
		out = append(out, s)
	}
	return out
}

// FormatStallResult renders a StallResult as a human-readable
// report with the verdict on the first line so a CI consumer can
// `head -1` it.
func FormatStallResult(r *StallResult) string {
	if r == nil {
		return "(nil stall result)"
	}
	var b strings.Builder
	if r.Stalled {
		fmt.Fprintf(&b, "STALLED — no node advanced validated_seq in %ds window\n",
			r.WindowSeconds)
	} else {
		fmt.Fprintf(&b, "OK — at least one node advanced validated_seq in %ds window\n",
			r.WindowSeconds)
	}

	// Per-node table sorted by name.
	type row struct {
		node, state string
		first, last int
		delta       int
	}
	idx := make(map[string]*row, len(r.Last))
	for _, s := range r.First {
		idx[s.Node] = &row{node: s.Node, first: s.ValidatedSeq}
	}
	for _, s := range r.Last {
		if x, ok := idx[s.Node]; ok {
			x.last = s.ValidatedSeq
			x.state = s.ServerState
			x.delta = x.last - x.first
		}
	}
	rows := make([]*row, 0, len(idx))
	for _, x := range idx {
		rows = append(rows, x)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].node < rows[j].node })

	fmt.Fprintf(&b, "  %-12s %-12s %-8s %-8s %s\n", "node", "state", "first", "last", "Δseq")
	for _, x := range rows {
		marker := ""
		if x.delta == 0 {
			marker = "  ⏸"
		} else if x.delta < 0 {
			marker = "  ↓"
		}
		fmt.Fprintf(&b, "  %-12s %-12s %-8d %-8d %+d%s\n",
			x.node, x.state, x.first, x.last, x.delta, marker)
	}
	return b.String()
}
