package runners

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/accounts"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/crash"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/generator"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/oracle"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// SoakConfig extends the bounded Config with soak-specific knobs.
type SoakConfig struct {
	Config
	TxRate      float64 // submissions per second; 0 = uncapped
	RotateEvery int64   // tx successes between account-pool tier rotations
	// OnPeriodic, when non-nil, is called from the soak loop's periodic
	// block after the crash poller's tick. The argument is the current
	// successful-tx step counter — useful for chaos schedulers keyed by
	// step number. Nil-tolerant.
	OnPeriodic func(step int)
	// LiveStats, when non-nil, is the *Stats the runner mutates in-place.
	// Lets external callers (e.g. cmd/fuzz HTTP status) snapshot live
	// counters during the run rather than only after completion. Nil =
	// allocate locally.
	LiveStats *Stats
	// LivenessStallThreshold raises a consensus_stall divergence when no
	// node has advanced its validated_ledger.seq for this long. Zero
	// disables. Default 60s (set explicitly in cmd/fuzz config loader).
	LivenessStallThreshold time.Duration
	// LivenessMinPeers raises a peer_drop divergence when any node reports
	// fewer connected peers than this. Zero disables.
	LivenessMinPeers int
	// LivenessSampleInterval is the server_info poll cadence for the
	// liveness monitor. Default 5s.
	LivenessSampleInterval time.Duration
	// EnabledOracles selectively activates per-oracle code paths. Empty =
	// all implemented oracles enabled (state_diff, consensus_liveness,
	// peer_health). Comes from scenario.Oracles via the ORACLES env var.
	EnabledOracles []string
}

// oracleEnabled reports whether name is in cfg.EnabledOracles, treating an
// empty/nil slice as "all enabled".
func (c SoakConfig) oracleEnabled(name string) bool {
	if len(c.EnabledOracles) == 0 {
		return true
	}
	for _, o := range c.EnabledOracles {
		if o == name {
			return true
		}
	}
	return false
}

// SoakRun runs an unbounded fuzz loop until ctx is cancelled. It reuses the
// realtime helpers (pool, generator, oracle, recorder) but never returns
// based on a tx count. Account-tier rotation is stubbed here; see C2.
func SoakRun(ctx context.Context, cfg SoakConfig) (*Stats, error) {
	if len(cfg.NodeURLs) < 2 {
		return nil, fmt.Errorf("need >= 2 NodeURLs")
	}
	submit := rpcclient.New(cfg.SubmitURL)
	nodes := make([]oracle.Node, len(cfg.NodeURLs))
	for i, u := range cfg.NodeURLs {
		nodes[i] = oracle.Node{Name: nodeName(u), Client: rpcclient.New(u)}
	}
	orc := oracle.New(nodes)
	rec := corpus.NewRecorder(cfg.CorpusDir, cfg.Seed).WithMirrorDir(cfg.FindingsMirrorDir)
	txLog, err := corpus.NewRunLog(cfg.CorpusDir, cfg.Seed)
	if err != nil {
		return nil, fmt.Errorf("run log: %w", err)
	}
	defer txLog.Close()

	pool, err := accounts.NewPool(cfg.Seed, cfg.AccountN)
	if err != nil {
		return nil, err
	}
	rng := corpus.NewRNG(cfg.Seed)
	accounts.AssignTiers(pool, cfg.TierWeights, rng.Rand())

	if !cfg.SkipFund {
		if err := accounts.FundFromGenesis(submit, pool, 10_000_000_000); err != nil {
			recordSetupFailure(rec, cfg.Metrics, cfg.Alerter, "soak", "fund", err)
			return nil, fmt.Errorf("fund: %w", err)
		}
		time.Sleep(5 * time.Second)
	}

	var enabled []string
	if !cfg.SkipSetup {
		if err := accounts.SetupState(submit, pool); err != nil {
			recordSetupFailure(rec, cfg.Metrics, cfg.Alerter, "soak", "setup_state", err)
			return nil, fmt.Errorf("setup state: %w", err)
		}
		enabled, err = generator.DiscoverEnabledAmendments(submit)
		if err != nil {
			recordSetupFailure(rec, cfg.Metrics, cfg.Alerter, "soak", "discover_amendments", err)
			return nil, err
		}
	}
	gen := generator.New(pool)

	stats := cfg.LiveStats
	if stats == nil {
		stats = &Stats{}
	}
	stats.Seed = cfg.Seed

	// Liveness monitor: detects consensus stalls + peer drops while the tx
	// submission loop runs. Fires divergences into the recorder + metrics
	// and publishes per-node network gauges every sample tick. Honors the
	// scenario.Oracles allowlist — disables stall detection when
	// consensus_liveness isn't requested, peer-drop when peer_health isn't.
	livenessWanted := cfg.oracleEnabled("consensus_liveness")
	peerHealthWanted := cfg.oracleEnabled("peer_health")
	stallThreshold := cfg.LivenessStallThreshold
	minPeers := cfg.LivenessMinPeers
	if !livenessWanted {
		stallThreshold = 0
	}
	if !peerHealthWanted {
		minPeers = 0
	}
	if stallThreshold > 0 || minPeers > 0 {
		go orc.Monitor(ctx, oracle.LivenessConfig{
			SampleInterval:   cfg.LivenessSampleInterval,
			StallThreshold:   stallThreshold,
			MinExpectedPeers: minPeers,
			OnSample: func(snaps []oracle.NodeSnapshot, stallSec float64) {
				if cfg.Metrics == nil {
					return
				}
				cfg.Metrics.NetworkStallSeconds.Set(stallSec)
				for _, s := range snaps {
					if s.Err != "" {
						continue
					}
					cfg.Metrics.NodeValidatedSeq.WithLabelValues(s.Name).Set(float64(s.ValidatedSeq))
					cfg.Metrics.NodePeerCount.WithLabelValues(s.Name).Set(float64(s.PeerCount))
					cfg.Metrics.NodeLastCloseConverge.WithLabelValues(s.Name).Set(s.LastCloseConvergeSec)
				}
			},
			OnEvent: func(e *oracle.LivenessEvent) {
				atomic.AddInt64(&stats.Divergences, 1)
				_, _ = rec.RecordDivergence(&corpus.Divergence{
					Kind:        e.Kind,
					Description: e.Description,
					Details: map[string]any{
						"stall_seconds": e.StallSeconds,
						"snapshots":     e.Snapshots,
					},
				})
				cfg.Alerter.Maybe(corpus.Signature(&corpus.Divergence{Kind: e.Kind, Description: e.Description}).Key(),
					fmt.Sprintf("[%s] %s", e.Kind, e.Description))
				if cfg.Metrics != nil {
					cfg.Metrics.Divergences.WithLabelValues(e.Kind).Inc()
				}
			},
		})
	}

	var poller *crash.Poller
	var hang *crash.HangDetector
	if cfg.CrashRuntime != nil && cfg.CrashLabelVal != "" {
		tail := cfg.CrashTailLines
		if tail == 0 {
			tail = 200
		}
		poller = crash.NewPoller(cfg.CrashRuntime, cfg.CrashLabelKey, cfg.CrashLabelVal, tail)
		poller.OnCrash = func(e *crash.Event) {
			atomic.AddInt64(&stats.Divergences, 1)
			_, _ = rec.RecordDivergence(&corpus.Divergence{
				Kind:        "crash",
				Description: fmt.Sprintf("%s exited %d (%s)", e.Container, e.ExitCode, e.Kind),
				Details: map[string]any{
					"container":   e.Container,
					"exit_code":   e.ExitCode,
					"crash_kind":  e.Kind,
					"marker_line": e.MarkerLine,
					"log_tail":    e.LogTail,
				},
			})
			cfg.Alerter.Maybe("", fmt.Sprintf("crash: %s exited %d (%s)", e.Container, e.ExitCode, e.Kind))
			if cfg.Metrics != nil {
				cfg.Metrics.Crashes.WithLabelValues(e.Container, e.Kind).Inc()
				cfg.Metrics.Divergences.WithLabelValues("crash").Inc()
			}
		}
		hang = crash.NewHangDetector(60)
		hang.Match = func(name string) bool { return strings.HasPrefix(name, "goxrpl-") }
		hang.Liveness = func(ctx context.Context, name string) (int64, error) {
			for _, n := range nodes {
				if n.Name == name {
					info, err := n.Client.ServerInfo()
					if err != nil {
						return 0, err
					}
					return int64(info.ValidatedLedger.Seq), nil
				}
			}
			return 0, fmt.Errorf("unknown node %q", name)
		}
	}

	var ticker *time.Ticker
	if cfg.TxRate > 0 {
		ticker = time.NewTicker(time.Duration(float64(time.Second) / cfg.TxRate))
		defer ticker.Stop()
	}

	var failLogSeq int64
	step := 0
	for {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return stats, nil
			}
			return stats, err
		}
		if ticker != nil {
			select {
			case <-ctx.Done():
				return stats, nil
			case <-ticker.C:
			}
		}

		tx, err := gen.PickTx(rng.Rand(), enabled)
		if err != nil {
			atomic.AddInt64(&stats.TxsFailed, 1)
			continue
		}
		txMode := "valid"
		if cfg.MutationRate > 0 {
			if mutated, did := gen.Mutator().Maybe(rng.Rand(), tx, cfg.MutationRate); did {
				tx = mutated
				txMode = "mutated"
				atomic.AddInt64(&stats.TxsMutated, 1)
			}
		}
		atomic.AddInt64(&stats.TxsSubmitted, 1)
		if cfg.Metrics != nil {
			cfg.Metrics.TxsSubmitted.WithLabelValues(tx.TransactionType(), txMode).Inc()
		}
		var res *rpcclient.SubmitResult
		if cfg.LocalSign {
			blob, signErr := submit.SignLocal(tx.Secret, tx.Fields)
			if signErr != nil {
				atomic.AddInt64(&stats.TxsFailed, 1)
				recordFailure(cfg.Metrics, txLog, 50, &failLogSeq, step,
					tx.TransactionType(), tx.Fields, tx.Secret, nil, signErr)
				continue
			}
			res, err = submit.SubmitTxBlob(blob)
		} else {
			res, err = submit.SubmitTxJSON(tx.Secret, tx.Fields)
		}
		if err != nil || (res.EngineResult != "tesSUCCESS" && res.EngineResult != "terQUEUED") {
			atomic.AddInt64(&stats.TxsFailed, 1)
			recordFailure(cfg.Metrics, txLog, 50, &failLogSeq, step,
				tx.TransactionType(), tx.Fields, tx.Secret, res, err)
			continue
		}
		atomic.AddInt64(&stats.TxsSucceeded, 1)
		if cfg.Metrics != nil {
			cfg.Metrics.TxsApplied.WithLabelValues(tx.TransactionType(), res.EngineResult).Inc()
		}
		_ = txLog.Append(&corpus.RunLogEntry{
			Step:   step,
			TxType: tx.TransactionType(),
			Fields: tx.Fields,
			Secret: tx.Secret,
			Result: res.EngineResult,
			TxHash: res.TxHash,
		})
		step++

		// Tracker feedback: record any object this tx created so the reference
		// tx types (EscrowFinish, OfferCancel, CheckCash, …) become eligible in
		// future picks. `submit` lets it discover minted NFTokenIDs.
		gen.RecordSuccess(tx, res.Sequence, submit)

		if res.TxHash != "" && cfg.oracleEnabled("state_diff") {
			if cmp := orc.CompareTxResult(ctx, res.TxHash); !cmp.Agreed {
				atomic.AddInt64(&stats.Divergences, 1)
				d := &corpus.Divergence{
					Kind:        "tx_result",
					Description: fmt.Sprintf("tx %s disagreed", res.TxHash),
					Details:     map[string]any{"tx_hash": res.TxHash, "tx_type": tx.TransactionType(), "node_results": cmp.NodeResults},
				}
				_, _ = rec.RecordDivergence(d)
				cfg.Alerter.Maybe(corpus.Signature(d).Key(), fmt.Sprintf("[%s] %s", d.Kind, d.Description))
				if cfg.Metrics != nil {
					cfg.Metrics.Divergences.WithLabelValues("tx_result").Inc()
				}
			}
			if meta := orc.CompareTxMetadata(ctx, res.TxHash); !meta.Agreed {
				atomic.AddInt64(&stats.Divergences, 1)
				d := &corpus.Divergence{
					Kind:        "metadata",
					Description: fmt.Sprintf("tx %s metadata diverged", res.TxHash),
					Details:     map[string]any{"tx_hash": res.TxHash, "tx_type": tx.TransactionType(), "node_meta": meta.NodeMeta},
				}
				_, _ = rec.RecordDivergence(d)
				cfg.Alerter.Maybe(corpus.Signature(d).Key(), fmt.Sprintf("[%s] %s", d.Kind, d.Description))
				if cfg.Metrics != nil {
					cfg.Metrics.Divergences.WithLabelValues("metadata").Inc()
				}
			}
		}
		if step%10 == 9 {
			if poller != nil {
				for _, n := range nodes {
					if hang.Step(ctx, n.Name) {
						log.Printf("soak: container %s appears hung — SIGQUIT", n.Name)
						_ = cfg.CrashRuntime.SendSignal(ctx, n.Name, "QUIT")
					}
				}
				_ = poller.Tick(ctx)
			}
			if cfg.Metrics != nil {
				if entries, err := os.ReadDir(filepath.Join(cfg.CorpusDir, "divergences")); err == nil {
					cfg.Metrics.CorpusSize.Set(float64(len(entries)))
				}
				if entries, err := os.ReadDir(filepath.Join(cfg.CorpusDir, "signatures")); err == nil {
					cfg.Metrics.UniqueSignatures.Set(float64(len(entries)))
				}
			}
			if cfg.OnPeriodic != nil {
				cfg.OnPeriodic(step)
			}
		}
		if cfg.RotateEvery > 0 && atomic.LoadInt64(&stats.TxsSucceeded)%cfg.RotateEvery == 0 {
			log.Printf("soak: rotating account tiers at %d successes", stats.TxsSucceeded)
			if err := accounts.RotateTiers(submit, pool, rng.Rand()); err != nil {
				log.Printf("soak: rotate: %v", err)
			}
		}
	}
}
