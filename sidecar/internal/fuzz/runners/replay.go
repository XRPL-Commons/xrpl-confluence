package runners

import (
	"context"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/accounts"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/mainnet"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/oracle"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// ReplayConfig drives ReplayRun. Field set mirrors Config but replaces
// MutationRate / TxCount with mainnet range parameters.
type ReplayConfig struct {
	NodeURLs    []string
	SubmitURL   string
	MainnetURL  string
	Seed        uint64
	AccountN    int
	LedgerStart int
	LedgerEnd   int
	CorpusDir   string
	BatchClose  time.Duration
	SkipFund    bool // escape hatch: skip genesis funding (unit tests)
	SkipSetup   bool // escape hatch: skip trust-line/IOU mesh seeding (unit tests)
}

// ReplayRun walks ledgers [LedgerStart, LedgerEnd] on MainnetURL, rewrites
// each tx against a seeded pool, and submits to SubmitURL. Oracle layers
// 1–4 run per the fuzz runner.
func ReplayRun(ctx context.Context, cfg ReplayConfig) (*Stats, error) {
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

	log.Printf("replay: seed=%#x accounts=%d range=%d..%d nodes=%d",
		cfg.Seed, cfg.AccountN, cfg.LedgerStart, cfg.LedgerEnd, len(cfg.NodeURLs))

	if !cfg.SkipFund {
		if err := accounts.FundFromGenesis(submit, pool, 10_000_000_000); err != nil {
			return nil, fmt.Errorf("fund pool: %w", err)
		}
		time.Sleep(5 * time.Second)
	}
	if !cfg.SkipSetup {
		if err := accounts.SetupState(submit, pool); err != nil {
			return nil, fmt.Errorf("setup state: %w", err)
		}
	}

	mc := mainnet.NewClient(cfg.MainnetURL)
	it := mainnet.NewIterator(mc, cfg.LedgerStart, cfg.LedgerEnd)
	rw := mainnet.NewRewriter(pool)

	info, err := submit.ServerInfo()
	if err != nil {
		return nil, fmt.Errorf("server_info: %w", err)
	}
	lastCompared := info.Validated.Seq

	var stats Stats
	stats.Seed = cfg.Seed
	i := 0

	for it.Next() {
		if err := ctx.Err(); err != nil {
			break
		}
		rawTx := it.Tx()
		out, ok, reason := rw.Rewrite(rawTx)
		if !ok {
			log.Printf("replay: skip (%s)", reason)
			atomic.AddInt64(&stats.TxsFailed, 1)
			continue
		}
		account, _ := out["Account"].(string)
		secret, found := rw.SecretFor(account)
		if !found {
			atomic.AddInt64(&stats.TxsFailed, 1)
			continue
		}

		atomic.AddInt64(&stats.TxsSubmitted, 1)
		res, err := submit.SubmitTxJSON(secret, out)
		if err != nil || (res.EngineResult != "tesSUCCESS" && res.EngineResult != "terQUEUED") {
			atomic.AddInt64(&stats.TxsFailed, 1)
			continue
		}
		atomic.AddInt64(&stats.TxsSucceeded, 1)
		txType, _ := out["TransactionType"].(string)
		_ = txLog.Append(&corpus.RunLogEntry{
			Step:   i,
			TxType: txType,
			Fields: out,
			Secret: secret,
			Result: res.EngineResult,
			TxHash: res.TxHash,
		})

		// Layer 2: compare result on all nodes once the tx is validated.
		if res.TxHash != "" {
			cmp := orc.CompareTxResult(ctx, res.TxHash)
			if !cmp.Agreed {
				atomic.AddInt64(&stats.Divergences, 1)
				_, _ = rec.RecordDivergence(&corpus.Divergence{
					Kind:        "tx_result",
					Description: fmt.Sprintf("tx %s disagreed (replay)", res.TxHash),
					Details: map[string]any{
						"tx_hash": res.TxHash, "node_results": cmp.NodeResults,
					},
				})
			}

			// Layer 3: cross-node metadata diff on the same tx.
			meta := orc.CompareTxMetadata(ctx, res.TxHash)
			if !meta.Agreed {
				atomic.AddInt64(&stats.Divergences, 1)
				_, _ = rec.RecordDivergence(&corpus.Divergence{
					Kind:        "metadata",
					Description: fmt.Sprintf("tx %s metadata diverged (replay)", res.TxHash),
					Details: map[string]any{
						"tx_hash": res.TxHash, "node_meta": meta.NodeMeta,
					},
				})
			}
		}

		// Periodically run layer-1 oracle (state hash) and layer-4 invariant.
		if cfg.BatchClose > 0 && i%10 == 9 {
			time.Sleep(cfg.BatchClose)
			info, err := submit.ServerInfo()
			if err == nil {
				for seq := lastCompared + 1; seq <= info.Validated.Seq; seq++ {
					cmp := orc.CompareAtSequence(ctx, seq)
					atomic.AddInt64(&stats.LedgersCompared, 1)
					if !cmp.Agreed {
						atomic.AddInt64(&stats.Divergences, 1)
						_, _ = rec.RecordDivergence(&corpus.Divergence{
							Kind:        "state_hash",
							Description: fmt.Sprintf("ledger %d diverged (replay)", seq),
							Details:     map[string]any{"comparison": cmp},
						})
					}
				}
				if err := inv.CheckLedger(submit); err != nil {
					atomic.AddInt64(&stats.Divergences, 1)
					_, _ = rec.RecordDivergence(&corpus.Divergence{
						Kind:        "invariant",
						Description: err.Error(),
						Details:     map[string]any{"invariant": "pool_balance_monotone"},
					})
				}
				lastCompared = info.Validated.Seq
			}
		}
		i++
	}
	if it.Err() != nil {
		return &stats, fmt.Errorf("iterator: %w", it.Err())
	}
	return &stats, nil
}
