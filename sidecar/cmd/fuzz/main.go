// Command fuzz is the xrpl-confluence fuzzer CLI. It reads its configuration
// from environment variables (same pattern as trafficgen) and runs the
// realtime runner against the confluence topology.
//
// MODE switches the binary between "fuzz" (default, synthetic generator) and
// "replay" (mainnet tx-shape replay). Replay adds these env vars:
//
//	MAINNET_URL          — public rippled JSON-RPC (default https://s1.ripple.com:51234)
//	REPLAY_LEDGER_START  — first ledger to replay (required in replay mode)
//	REPLAY_LEDGER_END    — last ledger to replay, inclusive (required)
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
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/runners"
)

func main() {
	flag.Parse()
	mode := envDefault("MODE", "fuzz")

	var statsMu sync.RWMutex
	var currentStats *runners.Stats
	go serveHTTP(&statsMu, &currentStats)

	ctx := context.Background()

	switch mode {
	case "fuzz":
		cfg, err := loadConfig()
		if err != nil {
			log.Fatalf("config: %v", err)
		}
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

	default:
		log.Fatalf("unknown MODE %q (want fuzz or replay)", mode)
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

	return &runners.Config{
		NodeURLs:     nodes,
		SubmitURL:    submit,
		Seed:         seed,
		AccountN:     accounts,
		TxCount:      txCount,
		CorpusDir:    corpusDir,
		BatchClose:   batchClose,
		MutationRate: mutationRate,
	}, nil
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
	State string         `json:"state"`
	Stats *runners.Stats `json:"stats"`
}

func serveHTTP(mu *sync.RWMutex, sp **runners.Stats) {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		defer mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		resp := statusResponse{State: "running", Stats: *sp}
		if *sp != nil {
			resp.State = "completed"
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	log.Println("HTTP results server on :8081")
	if err := http.ListenAndServe(":8081", mux); err != nil {
		log.Printf("http: %v", err)
	}
}
