// Package runners hosts the top-level fuzz loops. M1 ships only the realtime
// runner, which submits txs asynchronously while real consensus close the
// ledgers at their own cadence. It wires together account pool, generator,
// oracle layers 1+2, and corpus recorder.
package runners

import (
	"context"
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
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/metrics"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/oracle"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// Config is the runner's entire input surface.
type Config struct {
	NodeURLs     []string
	SubmitURL    string
	Seed         uint64
	AccountN     int
	TxCount      int
	CorpusDir    string
	BatchClose   time.Duration
	SkipFund     bool    // escape hatch: skip genesis funding (unit tests)
	SkipSetup    bool    // escape hatch: skip trust-line/IOU mesh seeding (unit tests)
	MutationRate float64 // 0..1; probability each generated tx is mutated
	// CrashRuntime, when non-nil, is polled once per BatchClose tick and
	// crash events are recorded as divergences (kind="crash"). Nil disables.
	CrashRuntime   crash.ContainerRuntime
	CrashLabelKey  string // e.g. "fuzzer.role"
	CrashLabelVal  string // e.g. "node"
	CrashTailLines int    // log lines to capture on crash (default 200)
	// Metrics, when non-nil, receives counter increments on submission,
	// applied result, divergence, and crash events. Nil disables all metrics.
	Metrics *metrics.Registry
}

// Stats summarises one run.
type Stats struct {
	Seed            uint64 `json:"seed"`
	TxsSubmitted    int64  `json:"txs_submitted"`
	TxsSucceeded    int64  `json:"txs_succeeded"`
	TxsFailed       int64  `json:"txs_failed"`
	TxsMutated      int64  `json:"txs_mutated"`
	Divergences     int64  `json:"divergences"`
	LedgersCompared int64  `json:"ledgers_compared"`
}

// Run executes the realtime fuzz loop to completion or until ctx is cancelled.
func Run(ctx context.Context, cfg Config) (*Stats, error) {
	if len(cfg.NodeURLs) < 2 {
		return nil, fmt.Errorf("need >= 2 NodeURLs for oracle comparison")
	}

	submit := rpcclient.New(cfg.SubmitURL)
	nodes := make([]oracle.Node, len(cfg.NodeURLs))
	for i, u := range cfg.NodeURLs {
		nodes[i] = oracle.Node{Name: nodeName(u), Client: rpcclient.New(u)}
	}
	orc := oracle.New(nodes)
	rec := corpus.NewRecorder(cfg.CorpusDir, cfg.Seed)

	var stats Stats
	stats.Seed = cfg.Seed

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
			_ = rec.RecordDivergence(&corpus.Divergence{
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
					return int64(info.Validated.Seq), nil
				}
			}
			return 0, fmt.Errorf("unknown node %q", name)
		}
	}

	txLog, err := corpus.NewRunLog(cfg.CorpusDir, cfg.Seed)
	if err != nil {
		return nil, fmt.Errorf("run log: %w", err)
	}
	defer txLog.Close()

	pool, err := accounts.NewPool(cfg.Seed, cfg.AccountN)
	if err != nil {
		return nil, fmt.Errorf("account pool: %w", err)
	}

	addrs := []string{accounts.GenesisAddress}
	for _, w := range pool.All() {
		addrs = append(addrs, w.ClassicAddress)
	}
	inv := oracle.NewInvariantPoolBalance(addrs)

	rng := corpus.NewRNG(cfg.Seed)

	log.Printf("realtime: seed=%#x accounts=%d txs=%d nodes=%d",
		cfg.Seed, cfg.AccountN, cfg.TxCount, len(cfg.NodeURLs))

	if !cfg.SkipFund {
		if err := accounts.FundFromGenesis(submit, pool, 10_000_000_000); err != nil {
			return nil, fmt.Errorf("fund pool: %w", err)
		}
		time.Sleep(5 * time.Second)
	}
	if !cfg.SkipSetup {
		log.Printf("realtime: seeding state mesh (%d accounts) ...", cfg.AccountN)
		if err := accounts.SetupState(submit, pool); err != nil {
			return nil, fmt.Errorf("setup state: %w", err)
		}
		log.Printf("realtime: state mesh seeded")
	}

	enabled, err := generator.DiscoverEnabledAmendments(submit)
	if err != nil {
		return nil, fmt.Errorf("amendments: %w", err)
	}
	log.Printf("realtime: %d amendments enabled", len(enabled))

	gen := generator.New(pool)

	info, err := submit.ServerInfo()
	if err != nil {
		return nil, fmt.Errorf("server_info: %w", err)
	}
	lastCompared := info.Validated.Seq

	for i := 0; i < cfg.TxCount; i++ {
		if err := ctx.Err(); err != nil {
			break
		}

		tx, err := gen.PickTx(rng.Rand(), enabled)
		if err != nil {
			atomic.AddInt64(&stats.TxsFailed, 1)
			log.Printf("realtime: generator: %v", err)
			continue
		}

		txMode := "valid"
		if cfg.MutationRate > 0 {
			if mutated, didMutate := gen.Mutator().Maybe(rng.Rand(), tx, cfg.MutationRate); didMutate {
				tx = mutated
				txMode = "mutated"
				atomic.AddInt64(&stats.TxsMutated, 1)
			}
		}

		atomic.AddInt64(&stats.TxsSubmitted, 1)
		if cfg.Metrics != nil {
			cfg.Metrics.TxsSubmitted.WithLabelValues(tx.TransactionType(), txMode).Inc()
		}
		res, err := submitTx(submit, tx)
		if err != nil || (res.EngineResult != "tesSUCCESS" && res.EngineResult != "terQUEUED") {
			atomic.AddInt64(&stats.TxsFailed, 1)
			if err != nil {
				log.Printf("realtime: submit %s: %v", tx.TransactionType(), err)
			} else {
				log.Printf("realtime: submit %s: %s (%s)", tx.TransactionType(), res.EngineResult, res.EngineResultMessage)
			}
			continue
		}
		atomic.AddInt64(&stats.TxsSucceeded, 1)
		if cfg.Metrics != nil {
			cfg.Metrics.TxsApplied.WithLabelValues(tx.TransactionType(), res.EngineResult).Inc()
		}
		_ = txLog.Append(&corpus.RunLogEntry{
			Step:   i,
			TxType: tx.TransactionType(),
			Fields: tx.Fields,
			Secret: tx.Secret,
			Result: res.EngineResult,
			TxHash: res.TxHash,
		})

		// Layer 2: compare result on all nodes once the tx is validated.
		if res.TxHash != "" {
			cmp := orc.CompareTxResult(ctx, res.TxHash)
			if !cmp.Agreed {
				atomic.AddInt64(&stats.Divergences, 1)
				_ = rec.RecordDivergence(&corpus.Divergence{
					Kind:        "tx_result",
					Description: fmt.Sprintf("tx %s disagreed across nodes", res.TxHash),
					Details: map[string]any{
						"tx_hash":      res.TxHash,
						"tx_type":      tx.TransactionType(),
						"node_results": cmp.NodeResults,
					},
				})
				if cfg.Metrics != nil {
					cfg.Metrics.Divergences.WithLabelValues("tx_result").Inc()
				}
			}
		}

		// Layer 3: cross-node metadata diff on the same tx.
		if res.TxHash != "" {
			meta := orc.CompareTxMetadata(ctx, res.TxHash)
			if !meta.Agreed {
				atomic.AddInt64(&stats.Divergences, 1)
				_ = rec.RecordDivergence(&corpus.Divergence{
					Kind:        "metadata",
					Description: fmt.Sprintf("tx %s metadata diverged", res.TxHash),
					Details: map[string]any{
						"tx_hash":   res.TxHash,
						"tx_type":   tx.TransactionType(),
						"node_meta": meta.NodeMeta,
					},
				})
				if cfg.Metrics != nil {
					cfg.Metrics.Divergences.WithLabelValues("metadata").Inc()
				}
			}
		}

		// Tracker feedback: on successful EscrowCreate, record (owner, sequence) so
		// EscrowFinish / EscrowCancel become eligible in future picks.
		if tx.TransactionType() == "EscrowCreate" && res.Sequence > 0 {
			if account, ok := tx.Fields["Account"].(string); ok {
				gen.Tracker().Escrows().Record(account, res.Sequence)
			}
		}

		// Periodically run layer-1 oracle.
		if cfg.BatchClose > 0 && i%10 == 9 {
			time.Sleep(cfg.BatchClose)
			if hang != nil {
				for _, n := range nodes {
					if hang.Step(ctx, n.Name) {
						log.Printf("realtime: container %s appears hung — SIGQUIT", n.Name)
						_ = cfg.CrashRuntime.SendSignal(ctx, n.Name, "QUIT")
					}
				}
			}
			if poller != nil {
				_ = poller.Tick(ctx)
			}
			info, err := submit.ServerInfo()
			if err == nil {
				for seq := lastCompared + 1; seq <= info.Validated.Seq; seq++ {
					cmp := orc.CompareAtSequence(ctx, seq)
					atomic.AddInt64(&stats.LedgersCompared, 1)
					if !cmp.Agreed {
						atomic.AddInt64(&stats.Divergences, 1)
						_ = rec.RecordDivergence(&corpus.Divergence{
							Kind:        "state_hash",
							Description: fmt.Sprintf("ledger %d diverged", seq),
							Details:     map[string]any{"comparison": cmp},
						})
						if cfg.Metrics != nil {
							cfg.Metrics.Divergences.WithLabelValues("state_hash").Inc()
						}
					}
				}
				lastCompared = info.Validated.Seq

				if err := inv.CheckLedger(submit); err != nil {
					atomic.AddInt64(&stats.Divergences, 1)
					_ = rec.RecordDivergence(&corpus.Divergence{
						Kind:        "invariant",
						Description: err.Error(),
						Details:     map[string]any{"invariant": "pool_balance_monotone"},
					})
					if cfg.Metrics != nil {
						cfg.Metrics.Divergences.WithLabelValues("invariant").Inc()
					}
				}
			}
			if cfg.Metrics != nil {
				if entries, err := os.ReadDir(filepath.Join(cfg.CorpusDir, "divergences")); err == nil {
					cfg.Metrics.CorpusSize.Set(float64(len(entries)))
				}
			}
		}
	}

	return &stats, nil
}

func nodeName(u string) string {
	name := strings.TrimPrefix(u, "http://")
	name = strings.TrimPrefix(name, "https://")
	if i := strings.Index(name, ":"); i > 0 {
		name = name[:i]
	}
	return name
}

// submitTx dispatches a Tx through the generic SubmitTxJSON path.
func submitTx(client *rpcclient.Client, tx *generator.Tx) (*rpcclient.SubmitResult, error) {
	return client.SubmitTxJSON(tx.Secret, tx.Fields)
}
