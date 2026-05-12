package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/finding"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/scenario"
)

const (
	RunStatusRunning          = "running"
	RunStatusCompletedBudget  = "completed_budget"
	RunStatusCompletedStopOn  = "completed_stop_on"
	RunStatusFailed           = "failed"
)

// stopOnMatches maps each stop_on value to the finding kinds that trigger it.
var stopOnMatches = map[string][]string{
	api.StopOnFirstDivergence: {api.KindStateDivergence},
	api.StopOnFirstCrash:      {api.KindNodeCrash},
	api.StopOnNone:            nil,
}

// Run is a single execution of a scenario tracked by the control service.
type Run struct {
	ID             string     `json:"id"`
	Scenario       string     `json:"scenario"`
	Status         string     `json:"status"`
	StartedAt      time.Time  `json:"started_at"`
	EndedAt        *time.Time `json:"ended_at,omitempty"`
	BudgetEndsAt   time.Time  `json:"budget_ends_at"`
	StopOn         []string   `json:"stop_on,omitempty"`
	FindingIDs     []string   `json:"finding_ids"`
	ReproducerIDs  []string   `json:"reproducer_ids,omitempty"`
	TriggerFinding string     `json:"trigger_finding,omitempty"`
}

// runStore is a thread-safe in-memory store for Run records.
type runStore struct {
	mu   sync.RWMutex
	runs map[string]*Run
}

func newRunStore() *runStore {
	return &runStore{runs: make(map[string]*Run)}
}

func (rs *runStore) add(r *Run) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.runs[r.ID] = r
}

func (rs *runStore) get(id string) (*Run, bool) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	r, ok := rs.runs[id]
	return r, ok
}

func (rs *runStore) list() []*Run {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	out := make([]*Run, 0, len(rs.runs))
	for _, r := range rs.runs {
		out = append(out, r)
	}
	return out
}

// StartRunRequest is the body for POST /v1/runs.
type StartRunRequest struct {
	Scenario api.Scenario `json:"scenario"`
}

// StartRunResponse is the body returned by POST /v1/runs.
type StartRunResponse struct {
	Run Run `json:"run"`
}

func (s *Server) startRun(w http.ResponseWriter, r *http.Request) {
	ct := r.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, api.ErrorResponse{
			Error: api.Error{Code: api.ErrCodeUnsupportedMediaType, Message: "Content-Type must be application/json"},
		})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req StartRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, api.ErrorResponse{
			Error: api.Error{Code: api.ErrCodeBadRequest, Message: err.Error()},
		})
		return
	}

	errs := scenario.Validate(&req.Scenario)
	if len(errs) > 0 {
		writeJSON(w, http.StatusBadRequest, api.ErrorResponse{
			Error: api.Error{
				Code:    api.ErrCodeScenarioInvalid,
				Message: errs[0].Message,
				Field:   errs[0].Field,
				Hint:    errs[0].Hint,
			},
		})
		return
	}

	dur, _ := time.ParseDuration(req.Scenario.Budget.Duration)

	run := &Run{
		ID:            finding.NewRunID(),
		Scenario:      req.Scenario.Metadata.Name,
		Status:        RunStatusRunning,
		StartedAt:     time.Now(),
		BudgetEndsAt:  time.Now().Add(dur),
		StopOn:        append([]string(nil), req.Scenario.Budget.StopOn...),
		FindingIDs:    []string{},
		ReproducerIDs: []string{},
	}

	s.runsMu.Lock()
	s.runs.add(run)
	s.currentRun = run
	s.runsMu.Unlock()

	go s.watchRun(run, req.Scenario)

	writeJSON(w, http.StatusOK, StartRunResponse{Run: *run})
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	all := s.runs.list()
	type listResp struct {
		Runs []*Run `json:"runs"`
	}
	out := make([]*Run, 0, len(all))
	for _, r := range all {
		cp := *r
		out = append(out, &cp)
	}
	writeJSON(w, http.StatusOK, listResp{Runs: out})
}

func (s *Server) runByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, ok := s.runs.get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, api.ErrorResponse{
			Error: api.Error{Code: api.ErrCodeRunNotFound, Message: "run not found", Field: "id"},
		})
		return
	}
	s.runs.mu.RLock()
	cp := *run
	s.runs.mu.RUnlock()
	writeJSON(w, http.StatusOK, cp)
}

func (s *Server) currentRunID() string {
	s.runsMu.RLock()
	defer s.runsMu.RUnlock()
	if s.currentRun == nil {
		return ""
	}
	return s.currentRun.ID
}

// watchRun monitors the run until budget elapses or a stop_on condition fires.
func (s *Server) watchRun(run *Run, sc api.Scenario) {
	if s.eventBus == nil {
		s.runBudgetLoop(run)
		return
	}

	subID, ch := s.eventBus.Subscribe(64)
	defer s.eventBus.Unsubscribe(subID)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if time.Now().After(run.BudgetEndsAt) {
				s.closeRun(run, RunStatusCompletedBudget, "")
				return
			}

		case ev, ok := <-ch:
			if !ok {
				return
			}
			if ev.Type != "finding" {
				continue
			}
			f, ok := ev.Payload.(api.Finding)
			if !ok {
				continue
			}
			s.runs.mu.Lock()
			run.FindingIDs = append(run.FindingIDs, f.ID)
			s.runs.mu.Unlock()

			if triggered, stopOnVal := s.matchesStopOn(run, f.Kind); triggered {
				_ = stopOnVal
				s.handleStopOnTrigger(run, sc, f)
				return
			}
		}
	}
}

// runBudgetLoop is the fallback used when no event bus is configured.
func (s *Server) runBudgetLoop(run *Run) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		if time.Now().After(run.BudgetEndsAt) {
			s.closeRun(run, RunStatusCompletedBudget, "")
			return
		}
	}
}

// matchesStopOn returns true if the finding kind triggers any of the run's stop_on conditions.
func (s *Server) matchesStopOn(run *Run, kind string) (bool, string) {
	s.runs.mu.RLock()
	stopOn := append([]string(nil), run.StopOn...)
	s.runs.mu.RUnlock()

	for _, cond := range stopOn {
		for _, k := range stopOnMatches[cond] {
			if k == kind {
				return true, cond
			}
		}
	}
	return false, ""
}

// handleStopOnTrigger closes the run, optionally emitting a reproducer.
func (s *Server) handleStopOnTrigger(run *Run, sc api.Scenario, trigger api.Finding) {
	s.closeRun(run, RunStatusCompletedStopOn, trigger.ID)

	if s.reproducerEmitter == nil {
		return
	}

	rep, err := s.reproducerEmitter.Emit(sc, &trigger)
	if err != nil {
		return
	}

	s.runs.mu.Lock()
	run.ReproducerIDs = append(run.ReproducerIDs, rep.ID)
	s.runs.mu.Unlock()
}

func (s *Server) closeRun(run *Run, status, triggerID string) {
	now := time.Now()
	s.runs.mu.Lock()
	run.Status = status
	run.EndedAt = &now
	run.TriggerFinding = triggerID
	s.runs.mu.Unlock()

	if s.eventBus != nil {
		s.eventBus.Publish(Event{Type: "run_completed", Payload: *run, Ts: now.UnixMilli()})
	}
}
