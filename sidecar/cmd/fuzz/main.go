// Command fuzz is the xrpl-confluence fuzzer CLI. It reads its configuration
// from environment variables (same pattern as trafficgen) and runs the
// realtime runner against the confluence topology.
//
// MODE switches the binary between "fuzz" (default, synthetic generator),
// "replay" (mainnet tx-shape replay), "reproduce" (replay a saved run log),
// "shrink" (single-probe shrinker — replay a prefix and check whether the
// original divergence reproduces), "soak" (unbounded fuzz with optional
// rate limiting and account-tier rotation), and "chaos" (extends soak with
// a deterministic chaos schedule). Replay adds these env vars:
//
//	MAINNET_URL          — public rippled JSON-RPC (default https://s1.ripple.com:51234)
//	REPLAY_LEDGER_START  — first ledger to replay (required in replay mode)
//	REPLAY_LEDGER_END    — last ledger to replay, inclusive (required)
//
// reproduce mode:
//
//	REPRODUCE_LOG — path to ndjson run log to replay (required in reproduce mode)
//
// shrink mode:
//
//	SHRINK_LOG              — path to ndjson run log (required)
//	SHRINK_DIVERGENCE       — path to original divergence JSON (required; defines signature)
//	SHRINK_MAX_STEP         — inclusive prefix cap on RunLogEntry.Step (required, >= 0)
//	SHRINK_RETRIES          — extra in-probe re-checks before concluding "no match" (default 0)
//	SHRINK_VALIDATE_TIMEOUT — per-tx wait for validated:true on every node (default 60s)
//
// soak mode:
//
//	TX_RATE    — submissions per second; 0 = uncapped (default 0)
//	ROTATE_EVERY — tx successes between account-pool tier rotations (default 1000)
//
// chaos mode (extends soak):
//
//	CHAOS_SCHEDULE — JSON array of chaos events; see chaos.ParseSchedule.
//
// Common environment variables:
//
//	NODES         — comma-separated node URLs for oracle observation (required)
//	SUBMIT_URL    — node tx submissions go to (default: first NODES entry)
//	FUZZ_SEED     — uint64; missing → crypto-random (logged at start)
//	ACCOUNTS      — account pool size (default 10)
//	CORPUS_DIR    — divergence output directory (default /output/corpus)
//	BATCH_CLOSE   — duration between layer-1 batch checks (default 5s)
//
// Fuzz-only environment variables:
//
//	TX_COUNT      — total tx submissions (default 100)
//	MUTATION_RATE — float 0..1; probability each generated tx is mutated (default 0.0)
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/chaos"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/crash"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/metrics"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/runners"
)

func main() {
	flag.Parse()
	mode := envDefault("MODE", "fuzz")

	mreg := metrics.New()

	var statsMu sync.RWMutex
	var currentStats *runners.Stats
	var currentShrink *runners.ShrinkResult
	go serveHTTP(mreg, &statsMu, &currentStats, &currentShrink)

	ctx := context.Background()

	switch mode {
	case "fuzz":
		cfg, err := loadConfig()
		if err != nil {
			log.Fatalf("config: %v", err)
		}
		mreg.CurrentSeed.Set(float64(cfg.Seed))
		mreg.AccountsActive.Set(float64(cfg.AccountN))
		cfg.Metrics = mreg
		log.Printf("fuzz: seed=%d nodes=%d submit=%s tx_count=%d accounts=%d corpus=%s mutation_rate=%.2f",
			cfg.Seed, len(cfg.NodeURLs), cfg.SubmitURL, cfg.TxCount, cfg.AccountN, cfg.CorpusDir, cfg.MutationRate)
		stats, err := runners.Run(ctx, *cfg)
		if err != nil {
			log.Fatalf("run: %v", err)
		}
		statsMu.Lock()
		currentStats = stats
		statsMu.Unlock()
		blob, _ := json.MarshalIndent(stats, "", "  ")
		log.Printf("fuzz: done\n%s", blob)

	case "replay":
		rcfg, err := loadReplayConfig()
		if err != nil {
			log.Fatalf("replay config: %v", err)
		}
		log.Printf("replay: seed=%d nodes=%d submit=%s mainnet=%s range=%d..%d accounts=%d corpus=%s",
			rcfg.Seed, len(rcfg.NodeURLs), rcfg.SubmitURL, rcfg.MainnetURL,
			rcfg.LedgerStart, rcfg.LedgerEnd, rcfg.AccountN, rcfg.CorpusDir)
		stats, err := runners.ReplayRun(ctx, *rcfg)
		if err != nil {
			log.Fatalf("replay: %v", err)
		}
		statsMu.Lock()
		currentStats = stats
		statsMu.Unlock()
		blob, _ := json.MarshalIndent(stats, "", "  ")
		log.Printf("replay: done\n%s", blob)

	case "reproduce":
		rcfg, err := loadReproduceConfig()
		if err != nil {
			log.Fatalf("reproduce config: %v", err)
		}
		log.Printf("reproduce: nodes=%d submit=%s log=%s",
			len(rcfg.NodeURLs), rcfg.SubmitURL, rcfg.LogPath)
		stats, err := runners.Reproduce(ctx, *rcfg)
		if err != nil {
			log.Fatalf("reproduce: %v", err)
		}
		statsMu.Lock()
		currentStats = stats
		statsMu.Unlock()
		blob, _ := json.MarshalIndent(stats, "", "  ")
		log.Printf("reproduce: done\n%s", blob)

	case "shrink":
		scfg, err := loadShrinkConfig()
		if err != nil {
			log.Fatalf("shrink config: %v", err)
		}
		log.Printf("shrink: nodes=%d submit=%s log=%s div=%s max_step=%d",
			len(scfg.NodeURLs), scfg.SubmitURL, scfg.LogPath, scfg.DivergenceFile, scfg.MaxStep)
		res, err := runners.Shrink(ctx, *scfg)
		if err != nil {
			log.Fatalf("shrink: %v", err)
		}
		statsMu.Lock()
		currentShrink = res
		statsMu.Unlock()
		blob, _ := json.MarshalIndent(res, "", "  ")
		log.Printf("shrink: done\n%s", blob)

	case "soak":
		cfg, err := loadSoakConfig()
		if err != nil {
			log.Fatalf("soak config: %v", err)
		}
		mreg.CurrentSeed.Set(float64(cfg.Seed))
		mreg.AccountsActive.Set(float64(cfg.AccountN))
		cfg.Metrics = mreg
		log.Printf("soak: seed=%d nodes=%d submit=%s rate=%.2f rotate_every=%d",
			cfg.Seed, len(cfg.NodeURLs), cfg.SubmitURL, cfg.TxRate, cfg.RotateEvery)
		stats, err := runners.SoakRun(ctx, *cfg)
		if err != nil {
			log.Fatalf("soak: %v", err)
		}
		statsMu.Lock()
		currentStats = stats
		statsMu.Unlock()
		blob, _ := json.MarshalIndent(stats, "", "  ")
		log.Printf("soak: done\n%s", blob)

	case "chaos":
		cfg, err := loadChaosConfig()
		if err != nil {
			log.Fatalf("chaos config: %v", err)
		}
		mreg.CurrentSeed.Set(float64(cfg.Seed))
		mreg.AccountsActive.Set(float64(cfg.AccountN))
		cfg.Metrics = mreg
		log.Printf("chaos: seed=%d nodes=%d submit=%s rate=%.2f rotate_every=%d events=%d",
			cfg.Seed, len(cfg.NodeURLs), cfg.SubmitURL, cfg.TxRate, cfg.RotateEvery, len(cfg.Schedule))
		stats, chaosStats, err := runners.ChaosRun(ctx, *cfg)
		if err != nil {
			log.Fatalf("chaos: %v", err)
		}
		statsMu.Lock()
		currentStats = stats
		statsMu.Unlock()
		blob, _ := json.MarshalIndent(struct {
			Soak  *runners.Stats `json:"soak"`
			Chaos *chaos.Stats   `json:"chaos"`
		}{stats, chaosStats}, "", "  ")
		log.Printf("chaos: done\n%s", blob)

	default:
		log.Fatalf("unknown MODE %q (want fuzz, replay, reproduce, shrink, soak, or chaos)", mode)
	}

	// Keep HTTP server alive so Kurtosis can scrape the results endpoint.
	select {}
}

func loadConfig() (*runners.Config, error) {
	nodes := strings.Split(os.Getenv("NODES"), ",")
	if len(nodes) < 2 || nodes[0] == "" {
		return nil, fmtErr("NODES must list >= 2 comma-separated URLs")
	}
	for i, n := range nodes {
		n = strings.TrimSpace(n)
		if !strings.HasPrefix(n, "http") {
			n = "http://" + n
		}
		nodes[i] = n
	}
	submit := os.Getenv("SUBMIT_URL")
	if submit == "" {
		submit = nodes[0]
	} else if !strings.HasPrefix(submit, "http") {
		submit = "http://" + submit
	}

	seed := corpus.SeedFromEnv("FUZZ_SEED")
	txCount := envInt("TX_COUNT", 100)
	accounts := envInt("ACCOUNTS", 10)
	corpusDir := envDefault("CORPUS_DIR", "/output/corpus")
	batchClose := envDuration("BATCH_CLOSE", 5*time.Second)
	mutationRate := envFloat("MUTATION_RATE", 0.0)

	cfg := &runners.Config{
		NodeURLs:     nodes,
		SubmitURL:    submit,
		Seed:         seed,
		AccountN:     accounts,
		TxCount:      txCount,
		CorpusDir:    corpusDir,
		BatchClose:   batchClose,
		MutationRate: mutationRate,
	}

	if val := os.Getenv("CRASH_LABEL_VAL"); val != "" {
		rt, err := crash.NewDockerRuntime()
		if err != nil {
			log.Printf("fuzz: crash poller disabled — docker dial failed: %v", err)
		} else {
			cfg.CrashRuntime = rt
			cfg.CrashLabelKey = envDefault("CRASH_LABEL_KEY", "fuzzer.role")
			cfg.CrashLabelVal = val
			if n, err := strconv.Atoi(envDefault("CRASH_TAIL_LINES", "200")); err == nil {
				cfg.CrashTailLines = n
			}
		}
	}

	return cfg, nil
}

func loadReplayConfig() (*runners.ReplayConfig, error) {
	nodes := strings.Split(os.Getenv("NODES"), ",")
	if len(nodes) < 2 || nodes[0] == "" {
		return nil, fmtErr("NODES must list >= 2 comma-separated URLs")
	}
	for i, n := range nodes {
		n = strings.TrimSpace(n)
		if !strings.HasPrefix(n, "http") {
			n = "http://" + n
		}
		nodes[i] = n
	}
	submit := os.Getenv("SUBMIT_URL")
	if submit == "" {
		submit = nodes[0]
	} else if !strings.HasPrefix(submit, "http") {
		submit = "http://" + submit
	}

	mainnetURL := envDefault("MAINNET_URL", "https://s1.ripple.com:51234")
	start := envInt("REPLAY_LEDGER_START", 0)
	end := envInt("REPLAY_LEDGER_END", 0)
	if start <= 0 || end <= 0 || end < start {
		return nil, fmtErr("REPLAY_LEDGER_START and REPLAY_LEDGER_END required (>0, end>=start)")
	}

	return &runners.ReplayConfig{
		NodeURLs:    nodes,
		SubmitURL:   submit,
		MainnetURL:  mainnetURL,
		Seed:        corpus.SeedFromEnv("FUZZ_SEED"),
		AccountN:    envInt("ACCOUNTS", 10),
		LedgerStart: start,
		LedgerEnd:   end,
		CorpusDir:   envDefault("CORPUS_DIR", "/output/corpus"),
		BatchClose:  envDuration("BATCH_CLOSE", 5*time.Second),
	}, nil
}

func loadReproduceConfig() (*runners.ReproduceConfig, error) {
	nodes := strings.Split(os.Getenv("NODES"), ",")
	if len(nodes) < 2 || nodes[0] == "" {
		return nil, fmtErr("NODES must list >= 2 comma-separated URLs")
	}
	for i, n := range nodes {
		n = strings.TrimSpace(n)
		if !strings.HasPrefix(n, "http") {
			n = "http://" + n
		}
		nodes[i] = n
	}
	submit := os.Getenv("SUBMIT_URL")
	if submit == "" {
		submit = nodes[0]
	} else if !strings.HasPrefix(submit, "http") {
		submit = "http://" + submit
	}

	logPath := os.Getenv("REPRODUCE_LOG")
	if logPath == "" {
		return nil, fmtErr("REPRODUCE_LOG env var required (path to ndjson run log)")
	}

	return &runners.ReproduceConfig{
		NodeURLs:  nodes,
		SubmitURL: submit,
		LogPath:   logPath,
		CorpusDir: envDefault("CORPUS_DIR", "/output/corpus"),
	}, nil
}

func loadShrinkConfig() (*runners.ShrinkConfig, error) {
	nodes := strings.Split(os.Getenv("NODES"), ",")
	if len(nodes) < 2 || nodes[0] == "" {
		return nil, fmtErr("NODES must list >= 2 comma-separated URLs")
	}
	for i, n := range nodes {
		n = strings.TrimSpace(n)
		if !strings.HasPrefix(n, "http") {
			n = "http://" + n
		}
		nodes[i] = n
	}
	submit := os.Getenv("SUBMIT_URL")
	if submit == "" {
		submit = nodes[0]
	} else if !strings.HasPrefix(submit, "http") {
		submit = "http://" + submit
	}

	logPath := os.Getenv("SHRINK_LOG")
	if logPath == "" {
		return nil, fmtErr("SHRINK_LOG env var required (path to ndjson run log)")
	}
	divPath := os.Getenv("SHRINK_DIVERGENCE")
	if divPath == "" {
		return nil, fmtErr("SHRINK_DIVERGENCE env var required (path to original divergence JSON)")
	}
	maxStep := envInt("SHRINK_MAX_STEP", -1)
	if maxStep < 0 {
		return nil, fmtErr("SHRINK_MAX_STEP env var required (>= 0)")
	}

	return &runners.ShrinkConfig{
		NodeURLs:        nodes,
		SubmitURL:       submit,
		Seed:            corpus.SeedFromEnv("FUZZ_SEED"),
		AccountN:        envInt("ACCOUNTS", 10),
		LogPath:         logPath,
		DivergenceFile:  divPath,
		MaxStep:         maxStep,
		Retries:         envInt("SHRINK_RETRIES", 0),
		CorpusDir:       envDefault("CORPUS_DIR", "/output/corpus"),
		ValidateTimeout: envDuration("SHRINK_VALIDATE_TIMEOUT", 60*time.Second),
	}, nil
}

func loadSoakConfig() (*runners.SoakConfig, error) {
	base, err := loadConfig()
	if err != nil {
		return nil, err
	}
	rate := envFloat("TX_RATE", 0)
	rotate, _ := strconv.ParseInt(envDefault("ROTATE_EVERY", "1000"), 10, 64)
	return &runners.SoakConfig{
		Config:      *base,
		TxRate:      rate,
		RotateEvery: rotate,
	}, nil
}

func loadChaosConfig() (*runners.ChaosConfig, error) {
	soak, err := loadSoakConfig()
	if err != nil {
		return nil, err
	}
	rt, dockerErr := chaos.NewDockerNetworkRuntime()
	if dockerErr != nil {
		log.Printf("chaos: NetworkRuntime disabled — docker dial failed: %v", dockerErr)
		rt = nil
	}
	var asInterface chaos.NetworkRuntime
	if rt != nil {
		asInterface = rt
	}
	schedule, parseErr := chaos.ParseSchedule(os.Getenv("CHAOS_SCHEDULE"), asInterface)
	if parseErr != nil {
		return nil, fmt.Errorf("CHAOS_SCHEDULE: %w", parseErr)
	}
	return &runners.ChaosConfig{
		SoakConfig: *soak,
		Schedule:   schedule,
	}, nil
}

func envDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
func fmtErr(s string) error { return &stringErr{s} }

type stringErr struct{ s string }

func (e *stringErr) Error() string { return e.s }

type statusResponse struct {
	State  string                `json:"state"`
	Stats  *runners.Stats        `json:"stats,omitempty"`
	Shrink *runners.ShrinkResult `json:"shrink,omitempty"`
}

func serveHTTP(mreg *metrics.Registry, mu *sync.RWMutex, sp **runners.Stats, shp **runners.ShrinkResult) {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		defer mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		resp := statusResponse{State: "running", Stats: *sp, Shrink: *shp}
		if *sp != nil || *shp != nil {
			resp.State = "completed"
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle("/metrics", mreg.Handler())
	log.Println("HTTP results server on :8081")
	if err := http.ListenAndServe(":8081", mux); err != nil {
		log.Printf("http: %v", err)
	}
}
