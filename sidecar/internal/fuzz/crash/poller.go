package crash

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// errNotFound is returned by ContainerRuntime when a container name is unknown.
var errNotFound = errors.New("container not found")

// ContainerRuntime is the minimal interface the poller needs. The Docker
// implementation lives in dockerruntime.go; tests inject a fake.
type ContainerRuntime interface {
	ListByLabel(ctx context.Context, key, val string) ([]string, error)
	Inspect(ctx context.Context, name string) (running bool, exitCode int, err error)
	TailLogs(ctx context.Context, name string, lines int) ([]string, error)
	SendSignal(ctx context.Context, name, sig string) error
}

// Event carries one detected crash to the OnCrash callback.
type Event struct {
	Container  string   `json:"container"`
	ExitCode   int      `json:"exit_code"`
	Kind       string   `json:"kind"` // from Classification
	MarkerLine int      `json:"marker_line"`
	LogTail    []string `json:"log_tail"`
}

// Poller polls the runtime each Tick and fires OnCrash once per crashed
// container per crash event. Re-runs of the same container without a
// new exit do not re-fire.
type Poller struct {
	rt        ContainerRuntime
	labelKey  string
	labelVal  string
	tailLines int
	OnCrash   func(*Event)

	mu   sync.Mutex
	seen map[string]int // container -> last exitCode reported
}

// NewPoller constructs a Poller. It looks up containers labelled
// labelKey=labelVal and tails tailLines log lines on each detected crash.
func NewPoller(rt ContainerRuntime, labelKey, labelVal string, tailLines int) *Poller {
	return &Poller{
		rt:        rt,
		labelKey:  labelKey,
		labelVal:  labelVal,
		tailLines: tailLines,
		seen:      make(map[string]int),
	}
}

// Tick performs one round of inspection. Idempotent for already-reported crashes.
func (p *Poller) Tick(ctx context.Context) error {
	names, err := p.rt.ListByLabel(ctx, p.labelKey, p.labelVal)
	if err != nil {
		return fmt.Errorf("list: %w", err)
	}
	for _, name := range names {
		running, code, err := p.rt.Inspect(ctx, name)
		if err != nil {
			continue
		}
		if running || code == 0 {
			continue
		}
		p.mu.Lock()
		if last, ok := p.seen[name]; ok && last == code {
			p.mu.Unlock()
			continue
		}
		p.seen[name] = code
		p.mu.Unlock()

		tail, _ := p.rt.TailLogs(ctx, name, p.tailLines)
		cls := Classify(tail)
		if p.OnCrash != nil {
			p.OnCrash(&Event{
				Container:  name,
				ExitCode:   code,
				Kind:       cls.Kind,
				MarkerLine: cls.MarkerLine,
				LogTail:    tail,
			})
		}
	}
	return nil
}

// HangDetector tracks per-container liveness signals (e.g. validated_ledger.seq)
// and asks the runtime to send SIGQUIT to a hung container so a Go goroutine
// dump or rippled stack trace is written before the eventual exit.
type HangDetector struct {
	StaleTicks int                                                            // how many consecutive same-value ticks count as hung
	Liveness   func(ctx context.Context, name string) (signal int64, err error)
	Match      func(name string) bool // only call SIGQUIT on matching containers (e.g. goXRPL)
	last       map[string]int64
	stale      map[string]int
	fired      map[string]bool
	seen       map[string]bool
}

// NewHangDetector returns a detector that triggers after staleTicks consecutive
// unchanged liveness samples.
func NewHangDetector(staleTicks int) *HangDetector {
	return &HangDetector{
		StaleTicks: staleTicks,
		last:       map[string]int64{},
		stale:      map[string]int{},
		fired:      map[string]bool{},
		seen:       map[string]bool{},
	}
}

// Step samples liveness for one container and returns true if SIGQUIT should
// fire (and has not yet fired this run).
func (h *HangDetector) Step(ctx context.Context, name string) bool {
	if h.fired[name] || h.Match == nil || !h.Match(name) || h.Liveness == nil {
		return false
	}
	v, err := h.Liveness(ctx, name)
	if err != nil {
		return false
	}
	if !h.seen[name] {
		h.seen[name] = true
		h.last[name] = v
		h.stale[name] = 1
	} else if v == h.last[name] {
		h.stale[name]++
	} else {
		h.last[name] = v
		h.stale[name] = 1
	}
	if h.stale[name] >= h.StaleTicks {
		h.fired[name] = true
		return true
	}
	return false
}
