package runners

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Peersyst/xrpl-go/xrpl/wallet"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/crash"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/generator"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/oracle"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// seqClient is the subset of *rpcclient.Client the sequence manager needs.
// Declared as an interface so the advance/resync logic can be unit-tested
// against a fake.
type seqClient interface {
	SignLocal(secret string, tx map[string]any) (string, error)
	SubmitTxBlob(blob string) (*rpcclient.SubmitResult, error)
	AccountInfoAt(account, ledgerIndex string) (*rpcclient.AccountInfoResult, error)
	LedgerCurrentIndex() (uint32, error)
}

// seqManager hands out monotonic per-account transaction sequences so the
// fuzzer can sign locally and submit concurrently without the auto-fill
// sequence race that otherwise caps throughput at a few tx/account/ledger.
//
// Each account is serialized by its own lock (only one in-flight submit at a
// time, in sequence order) while distinct accounts proceed in parallel. The
// local counter advances only when the engine result CONSUMES the sequence
// (tesSUCCESS / tec* / terQUEUED); on a sequence-drift result
// (tefPAST_SEQ / terPRE_SEQ) the account is resynced from the OPEN ledger; on
// any other non-consuming failure the sequence is held so the next tx for that
// account reuses it (no gap). Because the open-ledger admission cap is sized
// above the per-account burst, transactions apply directly rather than
// queueing, so the open-ledger sequence stays authoritative.
type seqManager struct {
	// client is the canonical node for local signing and sequence resync —
	// AccountInfoAt/LedgerCurrentIndex always target it so the cached open-
	// ledger sequence is read from a single consistent view.
	client seqClient
	// submitters spreads SubmitTxBlob across all nodes round-robin so the
	// per-node RPC throughput is not the bottleneck; out-of-order arrivals at
	// a given node simply queue until contiguous. Falls back to [client].
	submitters []seqClient
	rr         atomic.Uint64

	mu    sync.Mutex
	locks map[string]*sync.Mutex
	next  map[string]uint32
	known map[string]bool
	addrs map[string]string // secret -> classic address cache

	lastLedger atomic.Uint32
}

func newSeqManager(client seqClient, submitters []seqClient) *seqManager {
	if len(submitters) == 0 {
		submitters = []seqClient{client}
	}
	return &seqManager{
		client:     client,
		submitters: submitters,
		locks:      make(map[string]*sync.Mutex),
		next:       make(map[string]uint32),
		known:      make(map[string]bool),
		addrs:      make(map[string]string),
	}
}

// submitter returns the next round-robin node for blob submission.
func (s *seqManager) submitter() seqClient {
	if len(s.submitters) == 1 {
		return s.submitters[0]
	}
	i := s.rr.Add(1)
	return s.submitters[int(i)%len(s.submitters)]
}

// refreshLedger keeps the cached current-ledger index roughly current so
// LastLedgerSequence can be stamped without a per-tx ledger_current round trip.
func (s *seqManager) refreshLedger(ctx context.Context) {
	update := func() {
		if idx, err := s.client.LedgerCurrentIndex(); err == nil && idx != 0 {
			s.lastLedger.Store(idx)
		}
	}
	update()
	t := time.NewTicker(1500 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			update()
		}
	}
}

func (s *seqManager) lockFor(account string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.locks[account]
	if !ok {
		l = &sync.Mutex{}
		s.locks[account] = l
	}
	return l
}

// accountFor resolves the signing classic address for a tx, preferring an
// explicit Account field and falling back to deriving it from the secret
// (cached, since derivation is the same every call).
func (s *seqManager) accountFor(secret string, fields map[string]any) (string, error) {
	if a, ok := fields["Account"].(string); ok && a != "" {
		return a, nil
	}
	s.mu.Lock()
	a, ok := s.addrs[secret]
	s.mu.Unlock()
	if ok {
		return a, nil
	}
	w, err := wallet.FromSeed(secret, "")
	if err != nil {
		return "", err
	}
	a = w.ClassicAddress.String()
	s.mu.Lock()
	s.addrs[secret] = a
	s.mu.Unlock()
	return a, nil
}

func (s *seqManager) peek(account string) (uint32, error) {
	s.mu.Lock()
	if s.known[account] {
		seq := s.next[account]
		s.mu.Unlock()
		return seq, nil
	}
	s.mu.Unlock()

	// Resync against the open ledger so just-applied (unvalidated) txs are
	// reflected and we don't reissue a sequence already in flight.
	info, err := s.client.AccountInfoAt(account, "current")
	if err != nil {
		return 0, err
	}
	seq := uint32(info.Sequence)
	s.mu.Lock()
	s.next[account] = seq
	s.known[account] = true
	s.mu.Unlock()
	return seq, nil
}

func (s *seqManager) setNext(account string, seq uint32) {
	s.mu.Lock()
	s.next[account] = seq
	s.known[account] = true
	s.mu.Unlock()
}

func (s *seqManager) invalidate(account string) {
	s.mu.Lock()
	s.known[account] = false
	s.mu.Unlock()
}

// consumesSequence reports whether an engine result consumes the account
// sequence (so the local counter must advance). tesSUCCESS and every tec*
// claim consume sequence + fee; terQUEUED reserves the sequence and will apply.
func consumesSequence(engineResult string) bool {
	return engineResult == "tesSUCCESS" ||
		engineResult == "terQUEUED" ||
		strings.HasPrefix(engineResult, "tec")
}

// submitTx assigns the managed sequence for tx's signing account, signs the tx
// locally, and submits the blob. The account lock is held across the submit so
// sequences are issued strictly in order.
func (s *seqManager) submitTx(tx *generator.Tx) (*rpcclient.SubmitResult, error) {
	account, err := s.accountFor(tx.Secret, tx.Fields)
	if err != nil {
		return nil, err
	}

	l := s.lockFor(account)
	l.Lock()
	defer l.Unlock()

	seq, err := s.peek(account)
	if err != nil {
		return nil, err
	}

	tx.Fields["Account"] = account
	tx.Fields["Sequence"] = int(seq)
	if _, ok := tx.Fields["Fee"]; !ok {
		tx.Fields["Fee"] = "10"
	}
	if _, ok := tx.Fields["LastLedgerSequence"]; !ok {
		if ll := s.lastLedger.Load(); ll != 0 {
			tx.Fields["LastLedgerSequence"] = int(ll) + 50
		}
	}

	blob, err := s.client.SignLocal(tx.Secret, tx.Fields)
	if err != nil {
		s.invalidate(account) // re-fetch sequence next time
		return nil, err
	}
	res, err := s.submitter().SubmitTxBlob(blob)
	if err != nil {
		s.invalidate(account)
		return nil, err
	}

	switch {
	case consumesSequence(res.EngineResult):
		s.setNext(account, seq+1)
	case res.EngineResult == "tefPAST_SEQ" || res.EngineResult == "terPRE_SEQ":
		s.invalidate(account) // sequence drifted — resync from the open ledger
	default:
		// Non-consuming failure (tem*/tel*/other tef*): hold the sequence so
		// the next tx for this account reuses it, leaving no gap.
	}
	return res, nil
}

// soakDeps bundles the already-constructed soak dependencies so the concurrent
// path can reuse everything SoakRun sets up (pool, generator, oracle,
// recorder, crash poller) without re-deriving it.
type soakDeps struct {
	submit  *rpcclient.Client
	nodes   []oracle.Node
	rec     *corpus.Recorder
	txLog   *corpus.RunLog
	gen     *generator.Generator
	rng     *corpus.RNG
	stats   *Stats
	poller  *crash.Poller
	hang    *crash.HangDetector
	enabled []string
}

// runConcurrentSoak drives a high-throughput soak with client-side sequence
// management and a pool of submitter goroutines. A single coordinator goroutine
// owns all non-thread-safe state (generator, rng, run log, tracker feedback);
// the workers only touch the internally-synchronized seqManager, atomic stats,
// and the thread-safe metrics registry. Per-tx result/metadata oracles and
// account-tier rotation are intentionally disabled here — at thousands of
// tx/sec they cannot keep up, and the control-plane state_diff oracle plus the
// liveness monitor remain the divergence signal.
func runConcurrentSoak(ctx context.Context, cfg SoakConfig, d soakDeps) (*Stats, error) {
	workers := cfg.SubmitWorkers
	if workers < 1 {
		workers = 1
	}

	// Spread blob submission across every node's RPC; sign/resync stay on the
	// canonical submit node.
	submitters := make([]seqClient, 0, len(d.nodes))
	for _, n := range d.nodes {
		submitters = append(submitters, n.Client)
	}
	sm := newSeqManager(d.submit, submitters)
	bg, cancelBG := context.WithCancel(ctx)
	defer cancelBG()
	go sm.refreshLedger(bg)

	// Crash poll + corpus metrics on a timer, off the hot path. Touches none of
	// the generator/rng/pool state, so it is safe to run concurrently.
	if d.poller != nil || cfg.Metrics != nil {
		go maintenanceLoop(bg, cfg, d)
	}

	submitCh := make(chan *generator.Tx, workers*2)
	outcomeCh := make(chan submitOutcome, workers*2)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for tx := range submitCh {
				atomic.AddInt64(&d.stats.TxsSubmitted, 1)
				if cfg.Metrics != nil {
					cfg.Metrics.TxsSubmitted.WithLabelValues(tx.TransactionType(), "valid").Inc()
				}
				res, err := sm.submitTx(tx)
				outcomeCh <- submitOutcome{tx: tx, res: res, err: err}
			}
		}()
	}

	// Close outcomeCh once all workers have drained submitCh.
	go func() {
		wg.Wait()
		close(outcomeCh)
	}()

	var failLogSeq int64
	step := 0
	handle := func(o submitOutcome) {
		if o.err != nil || o.res == nil ||
			(o.res.EngineResult != "tesSUCCESS" && o.res.EngineResult != "terQUEUED") {
			atomic.AddInt64(&d.stats.TxsFailed, 1)
			recordFailure(cfg.Metrics, d.txLog, 50, &failLogSeq, step,
				o.tx.TransactionType(), o.tx.Fields, o.tx.Secret, o.res, o.err)
			return
		}
		atomic.AddInt64(&d.stats.TxsSucceeded, 1)
		if cfg.Metrics != nil {
			cfg.Metrics.TxsApplied.WithLabelValues(o.tx.TransactionType(), o.res.EngineResult).Inc()
		}
		_ = d.txLog.Append(&corpus.RunLogEntry{
			Step:   step,
			TxType: o.tx.TransactionType(),
			Fields: o.tx.Fields,
			Secret: o.tx.Secret,
			Result: o.res.EngineResult,
			TxHash: o.res.TxHash,
		})
		d.gen.RecordSuccess(o.tx, o.res.Sequence, d.submit)
		step++
	}

	var tickC <-chan time.Time
	if cfg.TxRate > 0 {
		ticker := time.NewTicker(time.Duration(float64(time.Second) / cfg.TxRate))
		defer ticker.Stop()
		tickC = ticker.C
	}

	for {
		if ctx.Err() != nil {
			break
		}
		if tickC != nil {
			select {
			case <-ctx.Done():
				goto drain
			case o := <-outcomeCh:
				handle(o)
				continue
			case <-tickC:
			}
		}

		tx, err := d.gen.PickTx(d.rng.Rand(), d.enabled)
		if err != nil {
			atomic.AddInt64(&d.stats.TxsFailed, 1)
			continue
		}
		if cfg.MutationRate > 0 {
			if mutated, did := d.gen.Mutator().Maybe(d.rng.Rand(), tx, cfg.MutationRate); did {
				tx = mutated
				atomic.AddInt64(&d.stats.TxsMutated, 1)
			}
		}

		sent := false
		for !sent {
			select {
			case submitCh <- tx:
				sent = true
			case o := <-outcomeCh:
				handle(o)
			case <-ctx.Done():
				goto drain
			}
		}
	}

drain:
	close(submitCh)
	for o := range outcomeCh {
		handle(o)
	}
	return d.stats, nil
}

// submitOutcome carries a completed submission back to the coordinator for
// stats, run-log, and tracker bookkeeping (all single-threaded there).
type submitOutcome struct {
	tx  *generator.Tx
	res *rpcclient.SubmitResult
	err error
}

// maintenanceLoop runs crash polling and corpus-size metrics on a timer for the
// concurrent soak path, mirroring the step%10 block of the serial loop minus
// the generator-coupled work (rotation, per-tx oracle).
func maintenanceLoop(ctx context.Context, cfg SoakConfig, d soakDeps) {
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if d.poller != nil && d.hang != nil {
				for _, n := range d.nodes {
					if d.hang.Step(ctx, n.Name) {
						_ = cfg.CrashRuntime.SendSignal(ctx, n.Name, "QUIT")
					}
				}
				_ = d.poller.Tick(ctx)
			}
			if cfg.Metrics != nil {
				if entries, err := os.ReadDir(filepath.Join(cfg.CorpusDir, "divergences")); err == nil {
					cfg.Metrics.CorpusSize.Set(float64(len(entries)))
				}
				if entries, err := os.ReadDir(filepath.Join(cfg.CorpusDir, "signatures")); err == nil {
					cfg.Metrics.UniqueSignatures.Set(float64(len(entries)))
				}
			}
		}
	}
}
