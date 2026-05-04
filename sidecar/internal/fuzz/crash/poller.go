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
