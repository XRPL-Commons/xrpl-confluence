package chaos

import (
	"context"
	"errors"
	"testing"
)

// stubEvent records Apply/Recover calls; used to exercise the Scheduler.
type stubEvent struct {
	name        string
	applyErr    error
	recoverErr  error
	applyCount  int
	recovCount  int
}

func (e *stubEvent) Name() string { return e.name }
func (e *stubEvent) Apply(ctx context.Context) error {
	e.applyCount++
	return e.applyErr
}
func (e *stubEvent) Recover(ctx context.Context) error {
	e.recovCount++
	return e.recoverErr
}

func TestScheduler_FiresAtTriggerStep(t *testing.T) {
	a := &stubEvent{name: "a"}
	b := &stubEvent{name: "b"}
	s := NewChaosScheduler([]ScheduleEntry{
		{TriggerStep: 5, Apply: a, RecoverAfter: 2},
		{TriggerStep: 10, Apply: b, RecoverAfter: 1},
	})

	for step := 0; step <= 12; step++ {
		s.Step(context.Background(), step)
	}

	if a.applyCount != 1 {
		t.Errorf("a applied %d times, want 1", a.applyCount)
	}
	if a.recovCount != 1 {
		t.Errorf("a recovered %d times, want 1", a.recovCount)
	}
	if b.applyCount != 1 {
		t.Errorf("b applied %d times, want 1", b.applyCount)
	}
	if b.recovCount != 1 {
		t.Errorf("b recovered %d times, want 1", b.recovCount)
	}

	stats := s.Stats()
	if stats.EventsApplied != 2 || stats.EventsRecovered != 2 {
		t.Errorf("stats = %+v", stats)
	}
}

func TestScheduler_PropagatesApplyError(t *testing.T) {
	bad := &stubEvent{name: "bad", applyErr: errors.New("nope")}
	s := NewChaosScheduler([]ScheduleEntry{
		{TriggerStep: 1, Apply: bad, RecoverAfter: 1},
	})

	s.Step(context.Background(), 1)

	stats := s.Stats()
	if stats.EventsApplied != 0 {
		t.Errorf("EventsApplied = %d, want 0 (apply failed)", stats.EventsApplied)
	}
	if stats.EventsErrored != 1 {
		t.Errorf("EventsErrored = %d, want 1", stats.EventsErrored)
	}

	if bad.recovCount != 0 {
		t.Errorf("recover fired despite Apply error: %d", bad.recovCount)
	}
}

func TestScheduler_NoEventsDoesNothing(t *testing.T) {
	s := NewChaosScheduler(nil)
	for step := 0; step < 100; step++ {
		s.Step(context.Background(), step)
	}
	stats := s.Stats()
	if stats.EventsApplied != 0 || stats.EventsRecovered != 0 || stats.EventsErrored != 0 {
		t.Errorf("expected zero stats, got %+v", stats)
	}
}
