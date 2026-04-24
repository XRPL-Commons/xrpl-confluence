// Package runners hosts the top-level fuzz loops. M1 ships only the realtime
// runner, which submits txs asynchronously while real consensus close the
// ledgers at their own cadence. It wires together account pool, generator,
// oracle layers 1+2, and corpus recorder.
package runners

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/accounts"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/generator"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/oracle"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// Config is the runner's entire input surface.
type Config struct {
	NodeURLs   []string
	SubmitURL  string
	Seed       uint64
	AccountN   int
	TxCount    int
	CorpusDir  string
	BatchClose time.Duration
	SkipFund   bool // escape hatch: skip genesis funding (unit tests)
	SkipSetup  bool // escape hatch: skip trust-line/IOU mesh seeding (unit tests)
}

// Stats summarises one run.
type Stats struct {
	Seed            uint64 `json:"seed"`
	TxsSubmitted    int64  `json:"txs_submitted"`
	TxsSucceeded    int64  `json:"txs_succeeded"`
	TxsFailed       int64  `json:"txs_failed"`
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
	rng := corpus.NewRNG(cfg.Seed)

	log.Printf("realtime: seed=%#x accounts=%d txs=%d nodes=%d",
		cfg.Seed, cfg.AccountN, cfg.TxCount, len(cfg.NodeURLs))

	pool, err := accounts.NewPool(cfg.Seed, cfg.AccountN)
	if err != nil {
		return nil, fmt.Errorf("account pool: %w", err)
	}
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

	var stats Stats
	stats.Seed = cfg.Seed

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

		atomic.AddInt64(&stats.TxsSubmitted, 1)
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
			}
		}

		// Periodically run layer-1 oracle.
		if cfg.BatchClose > 0 && i%10 == 9 {
			time.Sleep(cfg.BatchClose)
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
					}
				}
				lastCompared = info.Validated.Seq
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
