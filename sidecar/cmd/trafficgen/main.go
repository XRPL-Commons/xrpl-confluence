// Command trafficgen generates diverse XRPL transactions against a mixed
// rippled/goXRPL network and uses a hash oracle to verify all nodes agree on
// ledger state.
//
// Configuration via environment variables:
//
//	NODES         comma-separated name:port pairs (e.g. "rippled-0:5005,goxrpl-0:5005")
//	SUBMIT_NODE   name:port of the node to submit transactions to
//	TX_COUNT      total number of transactions to generate (default 100)
//	TX_MIX        type weights e.g. "payment:60,offer:20,trustset:10,accountset:10"
//	ACCOUNTS      number of test accounts to create (default 10)
//	LEDGER_WAIT   extra ledger closes to wait after last tx (default 5)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/oracle"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

const outputDir = "/output"

const (
	genesisAddress = "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"
	genesisSecret  = "snoPBrXtMeMyMHUVTgbuqAfg1SUTb"
)

// testAccount holds credentials for a funded test account.
type testAccount struct {
	Address string
	Secret  string
}

// txMix defines the weighted mix of transaction types.
type txMix struct {
	Payment    int
	Offer      int
	TrustSet   int
	AccountSet int
	total      int
}

// status tracks the overall progress of the traffic generator.
type status struct {
	mu            sync.RWMutex
	State         string                      `json:"status"`          // "starting", "running", "completed", "failed"
	TxSubmitted   int                         `json:"txs_submitted"`
	TxSucceeded   int                         `json:"txs_succeeded"`
	TxFailed      int                         `json:"txs_failed"`
	Divergences   int                         `json:"divergences"`
	Comparisons   []*oracle.LedgerComparison  `json:"comparisons"`
	StartedAt     time.Time                   `json:"started_at"`
	CompletedAt   *time.Time                  `json:"completed_at,omitempty"`
	Error         string                      `json:"error,omitempty"`
}

func (s *status) setRunning()  { s.mu.Lock(); s.State = "running"; s.mu.Unlock() }
func (s *status) setCompleted() {
	s.mu.Lock()
	s.State = "completed"
	now := time.Now()
	s.CompletedAt = &now
	s.mu.Unlock()
}
func (s *status) setFailed(err error) {
	s.mu.Lock()
	s.State = "failed"
	s.Error = err.Error()
	now := time.Now()
	s.CompletedAt = &now
	s.mu.Unlock()
}
func (s *status) addComparison(c *oracle.LedgerComparison) {
	s.mu.Lock()
	s.Comparisons = append(s.Comparisons, c)
	if !c.Agreed {
		s.Divergences++
	}
	s.mu.Unlock()
}
func (s *status) incSubmitted()          { s.mu.Lock(); s.TxSubmitted++; s.mu.Unlock() }
func (s *status) incSucceeded()          { s.mu.Lock(); s.TxSucceeded++; s.mu.Unlock() }
func (s *status) incFailed()             { s.mu.Lock(); s.TxFailed++; s.mu.Unlock() }

var currentStatus = &status{
	State:     "starting",
	StartedAt: time.Now(),
}

func main() {
	cfg := loadConfig()

	// Start the HTTP results server.
	go startHTTPServer()

	// Build node clients.
	nodes := buildNodes(cfg.nodeAddrs)
	submitClient := rpcclient.New("http://" + cfg.submitNode)

	// Build oracle.
	oracleNodes := make([]oracle.Node, len(nodes))
	for i, n := range nodes {
		oracleNodes[i] = oracle.Node{Name: n.name, Client: n.client}
	}
	orc := oracle.New(oracleNodes)

	ctx := context.Background()

	// Wait for the network to be live.
	log.Println("Waiting for network to be live...")
	waitForNetwork(ctx, nodes)
	log.Println("Network is live.")

	currentStatus.setRunning()

	// Get the starting validated sequence.
	info, err := submitClient.ServerInfo()
	if err != nil {
		currentStatus.setFailed(fmt.Errorf("server_info: %w", err))
		log.Fatalf("Failed to get server_info: %v", err)
	}
	startSeq := info.Validated.Seq
	log.Printf("Starting at validated seq %d", startSeq)

	// Create and fund test accounts.
	log.Printf("Creating %d test accounts...", cfg.accounts)
	testAccounts, err := createAccounts(submitClient, cfg.accounts)
	if err != nil {
		currentStatus.setFailed(fmt.Errorf("create accounts: %w", err))
		log.Fatalf("Failed to create accounts: %v", err)
	}
	log.Printf("Created %d test accounts", len(testAccounts))

	// Wait for funding to be validated.
	time.Sleep(5 * time.Second)

	// Ensure output directory exists for divergence captures.
	for _, subdir := range []string{"fixtures", "diagnostics"} {
		os.MkdirAll(filepath.Join(outputDir, subdir), 0o755)
	}

	// Track the last known-good sequence for divergence capture.
	lastGoodSeq := startSeq

	// Generate traffic.
	log.Printf("Generating %d transactions (mix: %+v)...", cfg.txCount, cfg.mix)
	generateTraffic(ctx, submitClient, orc, testAccounts, cfg, &lastGoodSeq)

	// Final oracle comparison across recent ledgers.
	log.Println("Running final oracle comparison...")
	finalInfo, err := submitClient.ServerInfo()
	if err == nil {
		endSeq := finalInfo.Validated.Seq
		for seq := max(startSeq+1, endSeq-5); seq <= endSeq; seq++ {
			compareAndCapture(ctx, orc, seq, &lastGoodSeq)
		}
	}

	currentStatus.setCompleted()

	currentStatus.mu.RLock()
	log.Printf("Done. Submitted=%d Succeeded=%d Failed=%d Divergences=%d",
		currentStatus.TxSubmitted, currentStatus.TxSucceeded, currentStatus.TxFailed, currentStatus.Divergences)
	currentStatus.mu.RUnlock()

	// Keep the HTTP server alive so Kurtosis can query results.
	select {}
}

// config holds parsed environment configuration.
type config struct {
	nodeAddrs  []string // e.g. ["rippled-0:5005", "goxrpl-0:5005"]
	submitNode string   // e.g. "rippled-0:5005"
	txCount    int
	accounts   int
	ledgerWait int
	mix        txMix
}

func loadConfig() config {
	cfg := config{
		txCount:    100,
		accounts:   10,
		ledgerWait: 5,
		mix:        txMix{Payment: 60, Offer: 20, TrustSet: 10, AccountSet: 10},
	}

	if v := os.Getenv("NODES"); v != "" {
		cfg.nodeAddrs = strings.Split(v, ",")
	} else {
		log.Fatal("NODES environment variable is required")
	}

	if v := os.Getenv("SUBMIT_NODE"); v != "" {
		cfg.submitNode = v
	} else {
		cfg.submitNode = cfg.nodeAddrs[0]
	}

	if v := os.Getenv("TX_COUNT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.txCount = n
		}
	}

	if v := os.Getenv("ACCOUNTS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.accounts = n
		}
	}

	if v := os.Getenv("LEDGER_WAIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.ledgerWait = n
		}
	}

	if v := os.Getenv("TX_MIX"); v != "" {
		cfg.mix = parseTxMix(v)
	}

	cfg.mix.total = cfg.mix.Payment + cfg.mix.Offer + cfg.mix.TrustSet + cfg.mix.AccountSet
	if cfg.mix.total == 0 {
		cfg.mix = txMix{Payment: 100, total: 100}
	}

	return cfg
}

func parseTxMix(s string) txMix {
	m := txMix{}
	for _, part := range strings.Split(s, ",") {
		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 {
			continue
		}
		weight, err := strconv.Atoi(kv[1])
		if err != nil {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(kv[0])) {
		case "payment":
			m.Payment = weight
		case "offer":
			m.Offer = weight
		case "trustset":
			m.TrustSet = weight
		case "accountset":
			m.AccountSet = weight
		}
	}
	return m
}

type namedClient struct {
	name   string
	client *rpcclient.Client
}

func buildNodes(addrs []string) []namedClient {
	nodes := make([]namedClient, len(addrs))
	for i, addr := range addrs {
		addr = strings.TrimSpace(addr)
		name := addr
		if idx := strings.Index(addr, ":"); idx > 0 {
			name = addr[:idx]
		}
		nodes[i] = namedClient{
			name:   name,
			client: rpcclient.New("http://" + addr),
		}
	}
	return nodes
}

func waitForNetwork(ctx context.Context, nodes []namedClient) {
	for _, n := range nodes {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			info, err := n.client.ServerInfo()
			if err == nil && info.Validated.Seq > 0 {
				log.Printf("  %s: validated_seq=%d", n.name, info.Validated.Seq)
				break
			}
			time.Sleep(2 * time.Second)
		}
	}
}

func createAccounts(submitClient *rpcclient.Client, count int) ([]testAccount, error) {
	accounts := make([]testAccount, 0, count)

	for i := 0; i < count; i++ {
		// Generate a new wallet.
		wallet, err := submitClient.WalletPropose()
		if err != nil {
			return nil, fmt.Errorf("wallet_propose: %w", err)
		}

		// Fund the account from genesis (10,000 XRP each).
		result, err := submitClient.SubmitPayment(
			genesisSecret,
			genesisAddress,
			wallet.AccountID,
			"10000000000", // 10,000 XRP in drops
		)
		if err != nil {
			return nil, fmt.Errorf("fund account %s: %w", wallet.AccountID, err)
		}

		if result.EngineResult != "tesSUCCESS" {
			log.Printf("  Warning: funding %s got %s: %s", wallet.AccountID, result.EngineResult, result.EngineResultMessage)
		}

		accounts = append(accounts, testAccount{
			Address: wallet.AccountID,
			Secret:  wallet.MasterSeed,
		})

		log.Printf("  Created account %d/%d: %s", i+1, count, wallet.AccountID)
	}

	return accounts, nil
}

func generateTraffic(ctx context.Context, submitClient *rpcclient.Client, orc *oracle.Oracle, accounts []testAccount, cfg config, lastGoodSeq *int) {
	batchSize := 10
	txGenerated := 0

	for txGenerated < cfg.txCount {
		// Generate a batch of transactions.
		batchEnd := min(txGenerated+batchSize, cfg.txCount)

		for txGenerated < batchEnd {
			txType := pickTxType(cfg.mix)
			var result *rpcclient.SubmitResult
			var err error

			switch txType {
			case "payment":
				result, err = generatePayment(submitClient, accounts)
			case "offer":
				result, err = generateOffer(submitClient, accounts)
			case "trustset":
				result, err = generateTrustSet(submitClient, accounts)
			case "accountset":
				result, err = generateAccountSet(submitClient, accounts)
			}

			currentStatus.incSubmitted()

			if err != nil {
				log.Printf("  tx %d/%d (%s): ERROR %v", txGenerated+1, cfg.txCount, txType, err)
				currentStatus.incFailed()
			} else if result.EngineResult == "tesSUCCESS" || result.EngineResult == "terQUEUED" {
				log.Printf("  tx %d/%d (%s): %s", txGenerated+1, cfg.txCount, txType, result.EngineResult)
				currentStatus.incSucceeded()
			} else {
				log.Printf("  tx %d/%d (%s): %s - %s", txGenerated+1, cfg.txCount, txType, result.EngineResult, result.EngineResultMessage)
				currentStatus.incFailed()
			}

			txGenerated++
		}

		// Wait for the batch to be validated, then run oracle comparison.
		time.Sleep(5 * time.Second)

		info, err := submitClient.ServerInfo()
		if err == nil && info.Validated.Seq > 0 {
			compareAndCapture(ctx, orc, info.Validated.Seq, lastGoodSeq)
			if info.Validated.Seq > *lastGoodSeq {
				*lastGoodSeq = info.Validated.Seq
			}
		}
	}
}

func pickTxType(m txMix) string {
	r := rand.Intn(m.total)
	if r < m.Payment {
		return "payment"
	}
	r -= m.Payment
	if r < m.Offer {
		return "offer"
	}
	r -= m.Offer
	if r < m.TrustSet {
		return "trustset"
	}
	return "accountset"
}

func generatePayment(client *rpcclient.Client, accounts []testAccount) (*rpcclient.SubmitResult, error) {
	if len(accounts) < 2 {
		return nil, fmt.Errorf("need at least 2 accounts for payment")
	}
	from := accounts[rand.Intn(len(accounts))]
	to := accounts[rand.Intn(len(accounts))]
	for to.Address == from.Address {
		to = accounts[rand.Intn(len(accounts))]
	}

	// Send 1-100 XRP.
	amount := fmt.Sprintf("%d", (rand.Intn(100)+1)*1000000)
	return client.SubmitPayment(from.Secret, from.Address, to.Address, amount)
}

func generateOffer(client *rpcclient.Client, accounts []testAccount) (*rpcclient.SubmitResult, error) {
	acct := accounts[rand.Intn(len(accounts))]

	// Create an offer: sell XRP for a synthetic IOU.
	takerPays := fmt.Sprintf("%d", (rand.Intn(100)+1)*1000000) // XRP drops
	takerGets := map[string]interface{}{
		"currency": "USD",
		"issuer":   accounts[0].Address, // Use first account as issuer.
		"value":    fmt.Sprintf("%d", rand.Intn(1000)+1),
	}

	return client.SubmitOfferCreate(acct.Secret, acct.Address, takerPays, takerGets)
}

func generateTrustSet(client *rpcclient.Client, accounts []testAccount) (*rpcclient.SubmitResult, error) {
	if len(accounts) < 2 {
		return nil, fmt.Errorf("need at least 2 accounts for trustset")
	}
	// Pick an account that's not the issuer.
	issuer := accounts[0]
	acct := accounts[1+rand.Intn(len(accounts)-1)]

	currencies := []string{"USD", "EUR", "GBP", "JPY", "BTC"}
	currency := currencies[rand.Intn(len(currencies))]
	limit := fmt.Sprintf("%d", (rand.Intn(10000)+1))

	return client.SubmitTrustSet(acct.Secret, acct.Address, currency, issuer.Address, limit)
}

func generateAccountSet(client *rpcclient.Client, accounts []testAccount) (*rpcclient.SubmitResult, error) {
	acct := accounts[rand.Intn(len(accounts))]

	// asfDefaultRipple = 8, asfRequireDest = 1, asfDisallowXRP = 3
	flags := []uint32{1, 3, 8}
	flag := flags[rand.Intn(len(flags))]

	return client.SubmitAccountSet(acct.Secret, acct.Address, flag)
}

// compareAndCapture runs the oracle comparison for a single ledger sequence.
// If a divergence is detected, it captures diagnostics and writes them to /output/.
func compareAndCapture(ctx context.Context, orc *oracle.Oracle, seq int, lastGoodSeq *int) {
	comp := orc.CompareAtSequence(ctx, seq)
	currentStatus.addComparison(comp)

	if comp.Agreed {
		log.Printf("  Oracle: seq %d AGREED", seq)
		*lastGoodSeq = seq
		return
	}

	log.Printf("  Oracle: seq %d DIVERGED: %v", seq, comp.Divergences)

	// Capture divergence diagnostics.
	capture := orc.CaptureDivergence(ctx, *lastGoodSeq, seq, comp)

	// Write diagnostics JSON.
	diagPath := filepath.Join(outputDir, "diagnostics", fmt.Sprintf("seq_%d.json", seq))
	if data, err := json.MarshalIndent(capture, "", "  "); err == nil {
		if err := os.WriteFile(diagPath, data, 0o644); err != nil {
			log.Printf("  Failed to write diagnostics: %v", err)
		} else {
			log.Printf("  Diagnostics written to %s", diagPath)
		}
	}

	// Write xrpl-fixtures format.
	fixturePath := filepath.Join(outputDir, "fixtures", fmt.Sprintf("divergence_seq_%d.json", seq))
	fixture := capture.ExportFixture()
	if data, err := json.MarshalIndent(fixture, "", "  "); err == nil {
		if err := os.WriteFile(fixturePath, data, 0o644); err != nil {
			log.Printf("  Failed to write fixture: %v", err)
		} else {
			log.Printf("  Fixture written to %s", fixturePath)
		}
	}
}

// HTTP results server.

func startHTTPServer() {
	mux := http.NewServeMux()

	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		currentStatus.mu.RLock()
		defer currentStatus.mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":      currentStatus.State,
			"divergences": currentStatus.Divergences,
		})
	})

	mux.HandleFunc("/results", func(w http.ResponseWriter, r *http.Request) {
		currentStatus.mu.RLock()
		defer currentStatus.mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(currentStatus)
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	log.Println("HTTP results server listening on :8081")
	if err := http.ListenAndServe(":8081", mux); err != nil {
		log.Printf("HTTP server error: %v", err)
	}
}
