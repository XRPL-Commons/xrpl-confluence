package server

import (
	"context"
	"sync"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/finding"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// NodeConfig is the static configuration for a single node endpoint.
type NodeConfig struct {
	Name string `json:"name"`
	Type string `json:"type"`
	RPC  string `json:"rpc"`
}

// LedgerRef mirrors rpcclient.LedgerRef for the HTTP API layer.
type LedgerRef struct {
	Seq  int    `json:"seq"`
	Hash string `json:"hash"`
}

// LastClose mirrors rpcclient.LastClose for the HTTP API layer.
type LastClose struct {
	Proposers     int     `json:"proposers"`
	ConvergeTimeS float64 `json:"converge_time_s"`
}

// Node is the per-node snapshot exposed by GET /v1/nodes.
type Node struct {
	Name               string     `json:"name"`
	Type               string     `json:"type"`
	Status             string     `json:"status"`
	ServerState        string     `json:"server_state,omitempty"`
	BuildVersion       string     `json:"build_version,omitempty"`
	Uptime             int        `json:"uptime,omitempty"`
	Peers              int        `json:"peers,omitempty"`
	CompleteLedgers    string     `json:"complete_ledgers,omitempty"`
	ValidatedLedger    *LedgerRef `json:"validated_ledger,omitempty"`
	ClosedLedger       *LedgerRef `json:"closed_ledger,omitempty"`
	LedgerCurrentIndex int        `json:"ledger_current_index,omitempty"`
	NetworkID          int        `json:"network_id,omitempty"`
	PubkeyNode         string     `json:"pubkey_node,omitempty"`
	LastClose          *LastClose `json:"last_close,omitempty"`
	Error              string     `json:"error,omitempty"`
}

// NodesResponse is the top-level body for GET /v1/nodes.
type NodesResponse struct {
	Timestamp int64  `json:"timestamp"` // unix millis
	Nodes     []Node `json:"nodes"`
}

// NodePoller polls each configured node on a fixed interval.
type NodePoller struct {
	cfgs     []NodeConfig
	interval time.Duration

	mu    sync.RWMutex
	nodes map[string]Node

	busMu    sync.RWMutex
	eventBus *EventBus

	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewNodePoller creates a NodePoller. Call Start to begin polling.
func NewNodePoller(cfg []NodeConfig, interval time.Duration) *NodePoller {
	nodes := make(map[string]Node, len(cfg))
	for _, c := range cfg {
		nodes[c.Name] = Node{Name: c.Name, Type: c.Type, Status: "unreachable"}
	}
	return &NodePoller{
		cfgs:     cfg,
		interval: interval,
		nodes:    nodes,
		stopCh:   make(chan struct{}),
	}
}

// Start launches one goroutine per node that polls on the configured interval.
// It stops when ctx is cancelled or Stop is called.
func (p *NodePoller) Start(ctx context.Context) {
	for _, cfg := range p.cfgs {
		cfg := cfg
		go p.pollLoop(ctx, cfg)
	}
}

// Stop signals all polling goroutines to exit.
func (p *NodePoller) Stop() {
	p.stopOnce.Do(func() { close(p.stopCh) })
}

// SetEventBus attaches an EventBus; a node snapshot is published after each poll.
func (p *NodePoller) SetEventBus(b *EventBus) {
	p.busMu.Lock()
	defer p.busMu.Unlock()
	p.eventBus = b
}

// Snapshot returns the latest aggregated state of all nodes.
func (p *NodePoller) Snapshot() NodesResponse {
	p.mu.RLock()
	defer p.mu.RUnlock()
	nodes := make([]Node, 0, len(p.cfgs))
	for _, cfg := range p.cfgs {
		nodes = append(nodes, p.nodes[cfg.Name])
	}
	return NodesResponse{
		Timestamp: time.Now().UnixMilli(),
		Nodes:     nodes,
	}
}

// DivergenceSnapshot satisfies finding.Snapshotter. It returns one entry per
// node that has a non-empty validated ledger.
func (p *NodePoller) DivergenceSnapshot() []finding.DivergenceInput {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]finding.DivergenceInput, 0, len(p.nodes))
	for _, n := range p.nodes {
		if n.ValidatedLedger != nil && n.ValidatedLedger.Hash != "" && n.ValidatedLedger.Seq > 0 {
			out = append(out, finding.DivergenceInput{
				Node: n.Name,
				Seq:  n.ValidatedLedger.Seq,
				Hash: n.ValidatedLedger.Hash,
			})
		}
	}
	return out
}

// ConsensusProgressSnapshot satisfies finding.ProgressSnapshotter. It returns
// one entry per node whose server_info poll has produced at least one ledger
// observation. Nodes still in "unreachable" state are skipped — they contribute
// neither closed nor validated info and would only generate noise.
//
// When a node reports closed but not validated (very early boot), validated_seq
// is left at 0; the stall oracle treats that as gap == closed and will fire if
// it stays that way past sustainFor, which is the intended signal.
func (p *NodePoller) ConsensusProgressSnapshot() []finding.ConsensusProgressInput {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]finding.ConsensusProgressInput, 0, len(p.nodes))
	for _, n := range p.nodes {
		if n.Status != "ok" {
			continue
		}
		var closed, validated int
		if n.ClosedLedger != nil {
			closed = n.ClosedLedger.Seq
		}
		if n.ValidatedLedger != nil {
			validated = n.ValidatedLedger.Seq
		}
		if closed == 0 && validated == 0 {
			continue
		}
		out = append(out, finding.ConsensusProgressInput{
			Node:         n.Name,
			ClosedSeq:    closed,
			ValidatedSeq: validated,
		})
	}
	return out
}

func (p *NodePoller) pollLoop(ctx context.Context, cfg NodeConfig) {
	p.poll(cfg)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.poll(cfg)
		}
	}
}

func (p *NodePoller) poll(cfg NodeConfig) {
	info, err := rpcclient.New(cfg.RPC).ServerInfo()

	p.mu.Lock()
	defer p.mu.Unlock()

	if err != nil {
		p.nodes[cfg.Name] = Node{
			Name:   cfg.Name,
			Type:   cfg.Type,
			Status: "unreachable",
			Error:  err.Error(),
		}
		return
	}

	n := Node{
		Name:               cfg.Name,
		Type:               cfg.Type,
		Status:             "ok",
		ServerState:        info.ServerState,
		BuildVersion:       info.BuildVersion,
		Uptime:             info.Uptime,
		Peers:              info.Peers,
		CompleteLedgers:    info.CompleteLedgers,
		LedgerCurrentIndex: info.LedgerCurrentIndex,
		NetworkID:          info.NetworkID,
		PubkeyNode:         info.PubkeyNode,
	}

	if info.ValidatedLedger.Seq > 0 || info.ValidatedLedger.Hash != "" {
		ref := LedgerRef{Seq: info.ValidatedLedger.Seq, Hash: info.ValidatedLedger.Hash}
		n.ValidatedLedger = &ref
	}
	if info.ClosedLedger.Seq > 0 || info.ClosedLedger.Hash != "" {
		ref := LedgerRef{Seq: info.ClosedLedger.Seq, Hash: info.ClosedLedger.Hash}
		n.ClosedLedger = &ref
	}
	if info.LastClose.Proposers > 0 || info.LastClose.ConvergeTimeS != 0 {
		lc := LastClose{Proposers: info.LastClose.Proposers, ConvergeTimeS: info.LastClose.ConvergeTimeS}
		n.LastClose = &lc
	}

	p.nodes[cfg.Name] = n

	p.busMu.RLock()
	bus := p.eventBus
	p.busMu.RUnlock()

	if bus != nil {
		snap := p.snapshotLocked()
		bus.Publish(Event{Type: "node", Payload: snap, Ts: snap.Timestamp})
	}
}

// snapshotLocked builds a NodesResponse while p.mu is already held.
func (p *NodePoller) snapshotLocked() NodesResponse {
	nodes := make([]Node, 0, len(p.cfgs))
	for _, cfg := range p.cfgs {
		nodes = append(nodes, p.nodes[cfg.Name])
	}
	return NodesResponse{Timestamp: time.Now().UnixMilli(), Nodes: nodes}
}
