package server

import (
	"net/http"
	"time"
)

// Option configures a Server.
type Option func(*Server)

// Server owns the HTTP mux and shared control-service state.
type Server struct {
	startedAt      time.Time
	scenario       string
	budgetDeadline time.Time
	mux            *http.ServeMux
}

// New creates a Server with the given options.
func New(opts ...Option) *Server {
	s := &Server{
		startedAt: time.Now(),
		mux:       http.NewServeMux(),
	}
	for _, o := range opts {
		o(s)
	}
	s.mux.HandleFunc("/v1/healthz", s.healthz)
	return s
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
