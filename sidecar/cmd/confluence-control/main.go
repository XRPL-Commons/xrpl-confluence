package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/server"
)

func main() {
	listen := flag.String("listen", ":8090", "address to listen on")
	scenario := flag.String("scenario", "", "display label for the active scenario")
	budgetDuration := flag.Duration("budget-duration", 0, "run budget duration; 0 = unbounded")
	flag.Parse()

	opts := []server.Option{}
	if *scenario != "" {
		opts = append(opts, server.WithScenario(*scenario))
	}
	if *budgetDuration > 0 {
		opts = append(opts, server.WithBudget(time.Now().Add(*budgetDuration)))
	}

	srv := &http.Server{
		Addr:    *listen,
		Handler: server.New(opts...).Handler(),
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
}
