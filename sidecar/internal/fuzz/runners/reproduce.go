package runners

import (
	"context"
	"fmt"
	"log"
	"sync/atomic"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/oracle"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// ReproduceConfig drives Reproduce. The caller is responsible for ensuring
// the topology is in a state compatible with the log's referenced accounts
// (typically: same seed, same fund+setup as the original run).
type ReproduceConfig struct {
	NodeURLs  []string
	SubmitURL string
	LogPath   string // path to the ndjson run log
	CorpusDir string // where to record any divergences observed during reproduction
}

// Reproduce reads an ndjson run log and re-submits each tx in order against
// SubmitURL, running oracle layers 2 (tx result) and 3 (metadata) after each
// successful submit. No setup / funding — the caller is responsible for
// topology state.
func Reproduce(ctx context.Context, cfg ReproduceConfig) (*Stats, error) {
	if len(cfg.NodeURLs) < 2 {
		return nil, fmt.Errorf("need >= 2 NodeURLs for oracle comparison")
	}
	entries, err := corpus.ReadRunLog(cfg.LogPath)
	if err != nil {
		return nil, fmt.Errorf("read run log: %w", err)
	}

	submit := rpcclient.New(cfg.SubmitURL)
	nodes := make([]oracle.Node, len(cfg.NodeURLs))
	for i, u := range cfg.NodeURLs {
		nodes[i] = oracle.Node{Name: nodeName(u), Client: rpcclient.New(u)}
	}
	orc := oracle.New(nodes)
	rec := corpus.NewRecorder(cfg.CorpusDir, 0)

	log.Printf("reproduce: %d entries from %s", len(entries), cfg.LogPath)

	var stats Stats
	for i, e := range entries {
		if err := ctx.Err(); err != nil {
			break
		}
		atomic.AddInt64(&stats.TxsSubmitted, 1)
		res, err := submit.SubmitTxJSON(e.Secret, e.Fields)
		if err != nil || (res.EngineResult != "tesSUCCESS" && res.EngineResult != "terQUEUED") {
			atomic.AddInt64(&stats.TxsFailed, 1)
			continue
		}
		atomic.AddInt64(&stats.TxsSucceeded, 1)

		if res.TxHash != "" {
			cmp := orc.CompareTxResult(ctx, res.TxHash)
			if !cmp.Agreed {
				atomic.AddInt64(&stats.Divergences, 1)
				_ = rec.RecordDivergence(&corpus.Divergence{
					Kind:        "tx_result",
					Description: fmt.Sprintf("tx %s disagreed at step %d (reproduce)", res.TxHash, i),
					Details: map[string]any{
						"step": i, "tx_type": e.TxType, "node_results": cmp.NodeResults,
					},
				})
			}
			meta := orc.CompareTxMetadata(ctx, res.TxHash)
			if !meta.Agreed {
				atomic.AddInt64(&stats.Divergences, 1)
				_ = rec.RecordDivergence(&corpus.Divergence{
					Kind:        "metadata",
					Description: fmt.Sprintf("tx %s metadata diverged at step %d (reproduce)", res.TxHash, i),
					Details: map[string]any{
						"step": i, "tx_type": e.TxType, "node_meta": meta.NodeMeta,
					},
				})
			}
		}
	}
	return &stats, nil
}
