package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/finding"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/server"
)

func main() {
	listen := flag.String("listen", ":8090", "address to listen on")
	scenario := flag.String("scenario", "", "display label for the active scenario")
	budgetDuration := flag.Duration("budget-duration", 0, "run budget duration; 0 = unbounded")
	nodesConfig := flag.String("nodes-config", "", "path to JSON file with node list")
	pollInterval := flag.Duration("poll-interval", 5*time.Second, "node poll interval")
	findingsDir := flag.String("findings-dir", "/var/confluence/findings", "directory watched for new findings")
	logsDir := flag.String("logs-dir", "/var/confluence/logs", "directory containing per-node log files")
	scenariosDir := flag.String("scenarios-dir", "/etc/confluence/scenarios", "directory containing built-in scenario YAML files")
	reproducersDir := flag.String("reproducers-dir", "/var/confluence/reproducers", "directory where reproducer scenario YAMLs are written")
	flag.Parse()

	if err := os.MkdirAll(*findingsDir, 0o755); err != nil {
		log.Fatalf("findings-dir: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus := server.NewEventBus()

	findingStore := finding.NewStore()
	findingStore.SetOnAdd(func(f api.Finding) {
		bus.Publish(server.Event{Type: "finding", Payload: f, Ts: f.OpenedAt.UnixMilli()})
	})
	// The runIDProvider is wired below after server.New so the watcher tags
	// each ingested divergence with the currently-active run_id.
	watcher := finding.NewDiskWatcher(*findingsDir, findingStore, 1*time.Second)

	emitter := server.NewReproducerEmitter(*reproducersDir, findingStore)

	opts := []server.Option{
		server.WithFindingStore(findingStore),
		server.WithLogsDir(*logsDir),
		server.WithEventBus(bus),
		server.WithScenariosDir(*scenariosDir),
		server.WithReproducerEmitter(emitter),
	}
	if *scenario != "" {
		opts = append(opts, server.WithScenario(*scenario))
	}
	if *budgetDuration > 0 {
		opts = append(opts, server.WithBudget(time.Now().Add(*budgetDuration)))
	}

	if *nodesConfig != "" {
		data, err := os.ReadFile(*nodesConfig)
		if err != nil {
			log.Fatalf("nodes-config: %v", err)
		}
		var file struct {
			Nodes []server.NodeConfig `json:"nodes"`
		}
		if err := json.Unmarshal(data, &file); err != nil {
			log.Fatalf("nodes-config parse: %v", err)
		}
		poller := server.NewNodePoller(file.Nodes, *pollInterval)
		poller.SetEventBus(bus)
		poller.Start(ctx)
		opts = append(opts, server.WithNodePoller(poller))

		oracle := finding.NewDivergenceOracle(poller, findingStore, 2*time.Second)
		oracle.Start(ctx)
	}

	confSrv := server.New(opts...)
	// Now that the server exists, give the disk watcher a way to look up
	// the active run_id at ingest time. This is what makes Run.FindingIDs
	// pick up corpus-mirrored divergences.
	watcher.WithRunIDProvider(confSrv.CurrentRunID)
	watcher.Start(ctx)

	srv := &http.Server{
		Addr:    *listen,
		Handler: confSrv.Handler(),
	}

	go func() {
		log.Printf("confluence-control listening on %s", *listen)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	cancel()

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
}
