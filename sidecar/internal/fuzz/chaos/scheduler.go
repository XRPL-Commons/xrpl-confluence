package chaos

import (
	"context"
	"log"
	"sync"
)

// Event is one disturbance the scheduler applies and later reverses.
// Apply must be idempotent if it returns an error — Recover only fires
// after an Apply that returned nil.
type Event interface {
	Name() string
	Apply(ctx context.Context) error
	Recover(ctx context.Context) error
}

// ScheduleEntry pairs an event with the soak-loop step at which Apply
// fires. Recover fires RecoverAfter steps later.
type ScheduleEntry struct {
	TriggerStep  int
	Apply        Event
	RecoverAfter int
}

// Stats summarises one chaos-scheduler run.
type Stats struct {
	EventsApplied   int64 `json:"events_applied"`
	EventsRecovered int64 `json:"events_recovered"`
	EventsErrored   int64 `json:"events_errored"`
}

// AuditEntry is what each fired event reports back to the soak loop so
// the runner can persist it as a corpus.Divergence (Kind "chaos") and
// tag downstream tx-level divergences with the event's identity.
type AuditEntry struct {
	Event string `json:"event"`
	Phase string `json:"phase"` // "apply" | "recover"
	Step  int    `json:"step"`
	Error string `json:"error,omitempty"`
}

// ChaosScheduler walks a fixed schedule; soak's periodic block calls
// Step(ctx, currentStep) once per tick.
type ChaosScheduler struct {
	mu       sync.Mutex
	schedule []ScheduleEntry
	pending  map[int][]*ScheduleEntry
	applied  map[*ScheduleEntry]bool
	stats    Stats
	OnAudit  func(AuditEntry)
}

// NewChaosScheduler constructs a scheduler from a sorted-or-unsorted slice;
// internally it's keyed by TriggerStep so order of input doesn't matter.
func NewChaosScheduler(schedule []ScheduleEntry) *ChaosScheduler {
	sched := make([]ScheduleEntry, len(schedule))
	copy(sched, schedule)
	return &ChaosScheduler{
		schedule: sched,
		pending:  map[int][]*ScheduleEntry{},
		applied:  map[*ScheduleEntry]bool{},
	}
}

// Step advances the scheduler one tick. The runner calls this from its
// existing periodic block (every N successful txs in soak).
func (s *ChaosScheduler) Step(ctx context.Context, step int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.schedule {
		e := &s.schedule[i]
		// Fire on the first tick at or after TriggerStep. The soak runner's
		// periodic block fires every N txs (currently every 10), so a strict
		// equality check would silently skip TriggerStep values that don't
		// align to the tick cadence.
		if s.applied[e] || step < e.TriggerStep {
			continue
		}
		if err := e.Apply.Apply(ctx); err != nil {
			s.stats.EventsErrored++
			s.emit(AuditEntry{Event: e.Apply.Name(), Phase: "apply", Step: step, Error: err.Error()})
			log.Printf("chaos: apply %s at step %d: %v", e.Apply.Name(), step, err)
			s.applied[e] = true
			continue
		}
		s.applied[e] = true
		s.stats.EventsApplied++
		s.emit(AuditEntry{Event: e.Apply.Name(), Phase: "apply", Step: step})
		log.Printf("chaos: apply %s at step %d", e.Apply.Name(), step)
		recoverAt := step + e.RecoverAfter
		s.pending[recoverAt] = append(s.pending[recoverAt], e)
	}

	if entries, ok := s.pending[step]; ok {
		for _, e := range entries {
			if err := e.Apply.Recover(ctx); err != nil {
				s.stats.EventsErrored++
				s.emit(AuditEntry{Event: e.Apply.Name(), Phase: "recover", Step: step, Error: err.Error()})
				log.Printf("chaos: recover %s at step %d: %v", e.Apply.Name(), step, err)
				continue
			}
			s.stats.EventsRecovered++
			s.emit(AuditEntry{Event: e.Apply.Name(), Phase: "recover", Step: step})
			log.Printf("chaos: recover %s at step %d", e.Apply.Name(), step)
		}
		delete(s.pending, step)
	}
}

// Stats returns the current running totals.
func (s *ChaosScheduler) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stats
}

func (s *ChaosScheduler) emit(a AuditEntry) {
	if s.OnAudit != nil {
		s.OnAudit(a)
	}
}
