package oracle

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// NodeSnapshot captures the liveness-relevant fields of a node at one sample.
type NodeSnapshot struct {
	Name                 string    `json:"name"`
	ValidatedSeq         int       `json:"validated_seq"`
	PeerCount            int       `json:"peer_count"`
	LastCloseConvergeSec float64   `json:"last_close_converge_sec"`
	ServerState          string    `json:"server_state"`
	SampledAt            time.Time `json:"sampled_at"`
	Err                  string    `json:"err,omitempty"`
}

// LivenessEvent describes a network-wide liveness violation.
type LivenessEvent struct {
	Kind         string         `json:"kind"`         // "consensus_stall" | "peer_drop"
	Description  string         `json:"description"`  // human-readable
	StallSeconds float64        `json:"stall_seconds,omitempty"`
	Snapshots    []NodeSnapshot `json:"snapshots"`
}

// LivenessConfig configures the consensus-liveness + peer-health monitor.
type LivenessConfig struct {
	// SampleInterval between server_info polls per node (default 5s).
	SampleInterval time.Duration
	// StallThreshold — duration with no validated-seq advance on ANY node
	// before raising a consensus_stall event. Default 60s.
	StallThreshold time.Duration
	// MinExpectedPeers — emit peer_drop when any node reports peers below this.
	// 0 disables the peer-health check.
	MinExpectedPeers int
	// OnEvent is invoked once per detected violation. Required.
	OnEvent func(*LivenessEvent)
	// OnSample, when non-nil, is invoked after every server_info poll with
	// the per-node snapshots and the wall-clock seconds since the network's
	// validated-ledger high-water last advanced (0 while progressing).
	// Lets callers update metrics gauges. Errors recorded in the snapshot
	// .Err field; non-failing call.
	OnSample func(snaps []NodeSnapshot, stallSeconds float64)
	// CooldownBetweenEvents debounces a flapping stall — after firing, suppress
	// repeats until cooldown elapses OR a validated-seq advance is observed
	// (whichever first). Default 30s.
	CooldownBetweenEvents time.Duration
}

// Monitor runs Liveness checks against the oracle's nodes until ctx is done.
// Blocking; intended to run in its own goroutine.
//
// Detection model — consensus_stall:
//   - At each tick, sample every node's ValidatedLedger.Seq.
//   - If NO node's seq has advanced since the last "high-water" sample for
//     StallThreshold, fire OnEvent with Kind="consensus_stall".
//   - The high-water resets whenever any node advances its seq.
//
// Detection model — peer_drop:
//   - At each tick, if any node reports Peers < MinExpectedPeers, fire OnEvent
//     with Kind="peer_drop" (subject to per-kind cooldown).
//
// Errors querying server_info are recorded in the snapshot but never bubbled
// up — the monitor is best-effort observability.
func (o *Oracle) Monitor(ctx context.Context, cfg LivenessConfig) {
	if cfg.OnEvent == nil {
		return
	}
	interval := cfg.SampleInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	stallThreshold := cfg.StallThreshold
	if stallThreshold <= 0 {
		stallThreshold = 60 * time.Second
	}
	cooldown := cfg.CooldownBetweenEvents
	if cooldown <= 0 {
		cooldown = 30 * time.Second
	}

	type state struct {
		mu               sync.Mutex
		highWaterSeq     int
		highWaterTime    time.Time
		lastStallFiredAt time.Time
		lastPeerFiredAt  time.Time
	}
	st := &state{highWaterTime: time.Now()}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		snapshots := o.sampleAll(ctx)

		// Determine max validated_seq across nodes.
		maxSeq := 0
		for _, s := range snapshots {
			if s.ValidatedSeq > maxSeq {
				maxSeq = s.ValidatedSeq
			}
		}

		st.mu.Lock()
		now := time.Now()

		// Consensus-stall detection.
		stallFor := time.Duration(0)
		if maxSeq > st.highWaterSeq {
			st.highWaterSeq = maxSeq
			st.highWaterTime = now
		} else {
			stallFor = now.Sub(st.highWaterTime)
		}
		// Publish a sample for metrics regardless of fire decision.
		if cfg.OnSample != nil {
			cfg.OnSample(snapshots, stallFor.Seconds())
		}
		if maxSeq <= st.highWaterSeq {
			if stallFor >= stallThreshold && now.Sub(st.lastStallFiredAt) >= cooldown {
				st.lastStallFiredAt = now
				st.mu.Unlock()
				cfg.OnEvent(&LivenessEvent{
					Kind:         "consensus_stall",
					Description:  fmt.Sprintf("no validated_seq advance across %d nodes for %.0fs (high-water seq=%d)", len(snapshots), stallFor.Seconds(), st.highWaterSeq),
					StallSeconds: stallFor.Seconds(),
					Snapshots:    snapshots,
				})
				continue
			}
		}

		// Peer-health detection.
		if cfg.MinExpectedPeers > 0 {
			var bad []NodeSnapshot
			for _, s := range snapshots {
				if s.Err == "" && s.PeerCount < cfg.MinExpectedPeers {
					bad = append(bad, s)
				}
			}
			if len(bad) > 0 && now.Sub(st.lastPeerFiredAt) >= cooldown {
				st.lastPeerFiredAt = now
				st.mu.Unlock()
				cfg.OnEvent(&LivenessEvent{
					Kind:        "peer_drop",
					Description: fmt.Sprintf("%d node(s) report peers < expected=%d", len(bad), cfg.MinExpectedPeers),
					Snapshots:   bad,
				})
				continue
			}
		}
		st.mu.Unlock()
	}
}

// sampleAll queries server_info on every node and returns parallel snapshots.
// Errors are recorded per-snapshot; the call always returns len(nodes) items.
func (o *Oracle) sampleAll(_ context.Context) []NodeSnapshot {
	out := make([]NodeSnapshot, len(o.nodes))
	var wg sync.WaitGroup
	for i, n := range o.nodes {
		i, n := i, n
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := NodeSnapshot{Name: n.Name, SampledAt: time.Now()}
			info, err := n.Client.ServerInfo()
			if err != nil {
				s.Err = err.Error()
				out[i] = s
				return
			}
			s.ValidatedSeq = info.ValidatedLedger.Seq
			s.PeerCount = info.Peers
			s.LastCloseConvergeSec = info.LastClose.ConvergeTimeS
			s.ServerState = info.ServerState
			out[i] = s
		}()
	}
	wg.Wait()
	return out
}
