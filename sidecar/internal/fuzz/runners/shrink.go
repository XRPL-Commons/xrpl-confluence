package runners

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/accounts"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/oracle"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// ShrinkConfig drives a single shrinker probe. The driver script supplies a
// fresh enclave per call, varying MaxStep to bisect the minimal failing prefix.
type ShrinkConfig struct {
	NodeURLs        []string
	SubmitURL       string
	Seed            uint64
	AccountN        int
	LogPath         string // ndjson run log to replay
	DivergenceFile  string // original divergence JSON (defines the matching signature)
	MaxStep         int    // inclusive prefix cap on RunLogEntry.Step
	Retries         int    // re-check this many extra times before concluding "no match" (default 0)
	CorpusDir       string
	ValidateTimeout time.Duration // per-tx wait for `validated:true`; 0 → 60s
	SkipFund        bool          // unit-test escape hatch
	SkipSetup       bool          // unit-test escape hatch
}

// ShrinkResult is the per-probe verdict written to <CorpusDir>/shrinks/.
type ShrinkResult struct {
	Matched      bool                       `json:"matched"`
	MaxStep      int                        `json:"max_step"`
	MatchedAt    int                        `json:"matched_at,omitempty"` // step at which the matching divergence was first observed
	Signature    corpus.DivergenceSignature `json:"signature"`
	Observed     []string                   `json:"observed,omitempty"` // kinds of all divergences seen this probe
	TxsSubmitted int64                      `json:"txs_submitted"`
	TxsSucceeded int64                      `json:"txs_succeeded"`
	DurationSecs float64                    `json:"duration_secs"`
}

// Shrink runs one probe: replays log entries with Step <= cfg.MaxStep against
// SubmitURL, waits for each tx to validate on every node, and reports whether
// any observed divergence matches the signature loaded from cfg.DivergenceFile.
//
// The caller is responsible for ensuring the topology is fresh — the shrinker
// runs FundFromGenesis + SetupState (unless skipped) but does not tear down
// any prior state.
func Shrink(ctx context.Context, cfg ShrinkConfig) (*ShrinkResult, error) {
	if len(cfg.NodeURLs) < 2 {
		return nil, fmt.Errorf("need >= 2 NodeURLs for oracle comparison")
	}
	if cfg.ValidateTimeout == 0 {
		cfg.ValidateTimeout = 60 * time.Second
	}

	sig, err := corpus.LoadDivergenceSignature(cfg.DivergenceFile)
	if err != nil {
		return nil, fmt.Errorf("signature: %w", err)
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

	if !cfg.SkipFund || !cfg.SkipSetup {
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
			if err := accounts.SetupState(submit, pool); err != nil {
				return nil, fmt.Errorf("setup state: %w", err)
			}
		}
	}

	res := &ShrinkResult{Signature: sig, MaxStep: cfg.MaxStep}
	start := time.Now()

	log.Printf("shrink: signature=%+v entries=%d max_step=%d", sig, len(entries), cfg.MaxStep)

	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			break
		}
		if e.Step > cfg.MaxStep {
			break
		}

		atomic.AddInt64(&res.TxsSubmitted, 1)
		out, err := submit.SubmitTxJSON(e.Secret, e.Fields)
		if err != nil || (out.EngineResult != "tesSUCCESS" && out.EngineResult != "terQUEUED") {
			continue
		}
		atomic.AddInt64(&res.TxsSucceeded, 1)
		if out.TxHash == "" {
			continue
		}

		// Wait for every node to validate before comparing — otherwise the
		// per-tx oracle is racy on real consensus.
		if err := orc.WaitTxValidated(ctx, out.TxHash, cfg.ValidateTimeout, 250*time.Millisecond); err != nil {
			log.Printf("shrink: validate %s: %v", out.TxHash, err)
			continue
		}

		if d := compareTxResultDivergence(orc, ctx, out.TxHash, e.TxType); d != nil {
			res.Observed = append(res.Observed, d.Kind)
			if !res.Matched && sig.Matches(d) {
				res.Matched = true
				res.MatchedAt = e.Step
			}
		}
		if d := compareTxMetadataDivergence(orc, ctx, out.TxHash, e.TxType); d != nil {
			res.Observed = append(res.Observed, d.Kind)
			if !res.Matched && sig.Matches(d) {
				res.Matched = true
				res.MatchedAt = e.Step
			}
		}
	}

	res.DurationSecs = time.Since(start).Seconds()

	if err := writeShrinkResult(cfg.CorpusDir, cfg.Seed, cfg.MaxStep, res); err != nil {
		log.Printf("shrink: write result: %v", err)
	}
	return res, nil
}

// compareTxResultDivergence returns a *Divergence iff layer-2 disagreed.
func compareTxResultDivergence(o *oracle.Oracle, ctx context.Context, hash, txType string) *corpus.Divergence {
	cmp := o.CompareTxResult(ctx, hash)
	if cmp.Agreed {
		return nil
	}
	return &corpus.Divergence{
		Kind: "tx_result",
		Details: map[string]any{
			"tx_hash":      hash,
			"tx_type":      txType,
			"node_results": cmp.NodeResults,
		},
	}
}

// compareTxMetadataDivergence returns a *Divergence iff layer-3 disagreed.
func compareTxMetadataDivergence(o *oracle.Oracle, ctx context.Context, hash, txType string) *corpus.Divergence {
	cmp := o.CompareTxMetadata(ctx, hash)
	if cmp.Agreed {
		return nil
	}
	return &corpus.Divergence{
		Kind: "metadata",
		Details: map[string]any{
			"tx_hash":   hash,
			"tx_type":   txType,
			"node_meta": cmp.NodeMeta,
		},
	}
}

func writeShrinkResult(corpusDir string, seed uint64, k int, res *ShrinkResult) error {
	dir := filepath.Join(corpusDir, "shrinks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, fmt.Sprintf("%x_k%d_result.json", seed, k))
	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
