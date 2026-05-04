package runners

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/accounts"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/generator"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/oracle"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// SoakConfig extends the bounded Config with soak-specific knobs.
// Account-tier rotation is wired in C2.
type SoakConfig struct {
	Config
	TxRate      float64 // submissions per second; 0 = uncapped
	RotateEvery int64   // tx successes between account-pool tier rotations (wired in C2)
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
	rec := corpus.NewRecorder(cfg.CorpusDir, cfg.Seed)
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

	if !cfg.SkipFund {
		if err := accounts.FundFromGenesis(submit, pool, 10_000_000_000); err != nil {
			return nil, fmt.Errorf("fund: %w", err)
		}
		time.Sleep(5 * time.Second)
	}

	var enabled []string
	if !cfg.SkipSetup {
		if err := accounts.SetupState(submit, pool); err != nil {
			return nil, fmt.Errorf("setup state: %w", err)
		}
		enabled, err = generator.DiscoverEnabledAmendments(submit)
		if err != nil {
			return nil, err
		}
	}
	gen := generator.New(pool)

	var stats Stats
	stats.Seed = cfg.Seed

	var ticker *time.Ticker
	if cfg.TxRate > 0 {
		ticker = time.NewTicker(time.Duration(float64(time.Second) / cfg.TxRate))
		defer ticker.Stop()
	}

	step := 0
	for {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return &stats, nil
			}
			return &stats, err
		}
		if ticker != nil {
			select {
			case <-ctx.Done():
				return &stats, nil
			case <-ticker.C:
			}
		}

		tx, err := gen.PickTx(rng.Rand(), enabled)
		if err != nil {
			atomic.AddInt64(&stats.TxsFailed, 1)
			continue
		}
		if cfg.MutationRate > 0 {
			if mutated, did := gen.Mutator().Maybe(rng.Rand(), tx, cfg.MutationRate); did {
				tx = mutated
				atomic.AddInt64(&stats.TxsMutated, 1)
			}
		}
		atomic.AddInt64(&stats.TxsSubmitted, 1)
		res, err := submit.SubmitTxJSON(tx.Secret, tx.Fields)
		if err != nil || (res.EngineResult != "tesSUCCESS" && res.EngineResult != "terQUEUED") {
			atomic.AddInt64(&stats.TxsFailed, 1)
			continue
		}
		atomic.AddInt64(&stats.TxsSucceeded, 1)
		_ = txLog.Append(&corpus.RunLogEntry{
			Step:   step,
			TxType: tx.TransactionType(),
			Fields: tx.Fields,
			Secret: tx.Secret,
			Result: res.EngineResult,
			TxHash: res.TxHash,
		})
		step++

		if res.TxHash != "" {
			if cmp := orc.CompareTxResult(ctx, res.TxHash); !cmp.Agreed {
				atomic.AddInt64(&stats.Divergences, 1)
				_ = rec.RecordDivergence(&corpus.Divergence{
					Kind:        "tx_result",
					Description: fmt.Sprintf("tx %s disagreed", res.TxHash),
					Details:     map[string]any{"tx_hash": res.TxHash, "node_results": cmp.NodeResults},
				})
			}
			if meta := orc.CompareTxMetadata(ctx, res.TxHash); !meta.Agreed {
				atomic.AddInt64(&stats.Divergences, 1)
				_ = rec.RecordDivergence(&corpus.Divergence{
					Kind:        "metadata",
					Description: fmt.Sprintf("tx %s metadata diverged", res.TxHash),
					Details:     map[string]any{"tx_hash": res.TxHash, "node_meta": meta.NodeMeta},
				})
			}
		}
		if cfg.RotateEvery > 0 && atomic.LoadInt64(&stats.TxsSucceeded)%cfg.RotateEvery == 0 {
			log.Printf("soak: rotating account tiers at %d successes", stats.TxsSucceeded)
			// TODO C2: rotate account tiers via accounts.RotateTiers(submit, pool, rng.Rand())
		}
	}
}
