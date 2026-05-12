package server

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/finding"
)

// Option configures a Server.
type Option func(*Server)

// Server owns the HTTP mux and shared control-service state.
type Server struct {
	startedAt      time.Time
	scenario       string
	budgetDeadline time.Time
	nodePoller     *NodePoller
	findingStore   *finding.Store
	eventBus       *EventBus
	logsDir        string
	scenariosDir   string
	mux            *http.ServeMux

	runs              *runStore
	runsMu            sync.RWMutex
	currentRun        *Run
	reproducerEmitter *ReproducerEmitter
}

// New creates a Server with the given options.
func New(opts ...Option) *Server {
	s := &Server{
		startedAt: time.Now(),
		mux:       http.NewServeMux(),
		runs:      newRunStore(),
	}
	for _, o := range opts {
		o(s)
	}
	s.mux.HandleFunc("/v1/healthz", s.healthz)
	if s.nodePoller != nil {
		s.mux.HandleFunc("/v1/nodes", s.nodes)
	}
	s.mux.HandleFunc("GET /v1/state/diff", s.stateDiff)
	s.mux.HandleFunc("GET /v1/findings", s.findingsList)
	s.mux.HandleFunc("GET /v1/findings/{id}", s.findingsByID)
	s.mux.HandleFunc("GET /v1/logs", s.logs)
	s.mux.HandleFunc("GET /v1/events", s.events)
	s.mux.HandleFunc("GET /v1/scenarios", s.scenariosList)
	s.mux.HandleFunc("POST /v1/scenarios/validate", s.scenariosValidate)
	s.mux.HandleFunc("POST /v1/runs", s.startRun)
	s.mux.HandleFunc("GET /v1/runs", s.listRuns)
	s.mux.HandleFunc("GET /v1/runs/{id}", s.runByID)
	return s
}

func (s *Server) nodes(w http.ResponseWriter, r *http.Request) {
	snap := s.nodePoller.Snapshot()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snap)
}

// Handler returns the configured HTTP handler.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// WithScenario sets the display label for the active scenario.
func WithScenario(name string) Option {
	return func(s *Server) { s.scenario = name }
}

// WithBudget sets the absolute deadline for the run budget.
func WithBudget(deadline time.Time) Option {
	return func(s *Server) { s.budgetDeadline = deadline }
}

// WithNodePoller attaches a NodePoller and registers GET /v1/nodes.
func WithNodePoller(p *NodePoller) Option {
	return func(s *Server) { s.nodePoller = p }
}

// WithFindingStore attaches a finding.Store for the /v1/findings endpoints.
func WithFindingStore(fs *finding.Store) Option {
	return func(s *Server) { s.findingStore = fs }
}

// WithLogsDir sets the directory from which per-node log files are read.
func WithLogsDir(dir string) Option {
	return func(s *Server) { s.logsDir = dir }
}

// WithEventBus attaches an EventBus used by the GET /v1/events SSE endpoint.
func WithEventBus(b *EventBus) Option {
	return func(s *Server) { s.eventBus = b }
}

// WithScenariosDir sets the directory scanned for built-in *.yaml scenario files.
func WithScenariosDir(dir string) Option {
	return func(s *Server) { s.scenariosDir = dir }
}

// WithReproducerEmitter attaches a ReproducerEmitter used when a stop_on run closes.
func WithReproducerEmitter(e *ReproducerEmitter) Option {
	return func(s *Server) { s.reproducerEmitter = e }
}
