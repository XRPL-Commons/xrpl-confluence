package chaos

import (
	"fmt"
	"math/rand"
	"strings"
)

// ScheduleEnv carries the information ParseSchedule needs to expand
// recurring/wildcard entries: the node-name universe (extracted from the
// sidecar's NODES env) and the run seed (used to derive a deterministic
// child RNG so expansion is reproducible).
type ScheduleEnv struct {
	Nodes []string
	Seed  uint64
}

// Recurring is a higher-level entry type that materialises many concrete
// rawEntry values during ParseSchedule. Use it to express "every N steps,
// fire this event, optionally with jitter and a randomised target."
type Recurring struct {
	Every     int      `json:"every"`
	StartStep int      `json:"start_step"`
	Count     int      `json:"count,omitempty"`
	UntilStep int      `json:"until_step,omitempty"`
	Jitter    int      `json:"jitter,omitempty"`
	Inner     rawEntry `json:"event"`
}

// ExpandRecurring materialises the entries the recurring spec describes.
// rng is a child RNG; callers should seed it deterministically from the
// run seed plus the recurring entry's position so the expansion is stable
// across re-runs of the same chaos schedule.
func ExpandRecurring(spec Recurring, env ScheduleEnv, rng *rand.Rand) ([]rawEntry, error) {
	if spec.Every <= 0 {
		return nil, fmt.Errorf("recurring: every must be > 0")
	}
	if spec.Count <= 0 && spec.UntilStep <= 0 {
		return nil, fmt.Errorf("recurring: count or until_step required")
	}
	if spec.Inner.Type == "" {
		return nil, fmt.Errorf("recurring: event.type required")
	}

	out := []rawEntry{}
	for i := 0; ; i++ {
		step := spec.StartStep + i*spec.Every
		if spec.UntilStep > 0 && step > spec.UntilStep {
			break
		}
		if spec.Count > 0 && i >= spec.Count {
			break
		}
		entry := spec.Inner
		if spec.Jitter > 0 {
			step += rng.Intn(2*spec.Jitter+1) - spec.Jitter
			if step < 0 {
				step = 0
			}
		}
		entry.Step = step

		if err := resolveWildcards(&entry, env, rng); err != nil {
			return nil, fmt.Errorf("recurring entry %d: %w", i, err)
		}

		if entry.Type == "latency" && entry.DelayMs == 0 {
			lo, hi := spec.Inner.DelayMsMin, spec.Inner.DelayMsMax
			if hi > 0 && lo > 0 && hi >= lo {
				entry.DelayMs = lo + rng.Intn(hi-lo+1)
			}
		}

		out = append(out, entry)
	}
	return out, nil
}

func resolveWildcards(entry *rawEntry, env ScheduleEnv, rng *rand.Rand) error {
	for _, p := range []*string{&entry.Container, &entry.From, &entry.To, &entry.Target} {
		if !strings.Contains(*p, "*") {
			continue
		}
		match := matchNodes(*p, env.Nodes)
		if len(match) == 0 {
			return fmt.Errorf("no nodes match pattern %q (nodes=%v)", *p, env.Nodes)
		}
		*p = match[rng.Intn(len(match))]
	}
	return nil
}

// matchNodes implements suffix-only wildcard matching. "rippled-*" matches
// any node whose name starts with "rippled-". Full glob (filepath.Match
// semantics) is over-engineered for predictable Kurtosis names where every
// service is named "<role>-<index>".
func matchNodes(pattern string, nodes []string) []string {
	if !strings.HasSuffix(pattern, "*") {
		return nil
	}
	prefix := strings.TrimSuffix(pattern, "*")
	out := []string{}
	for _, n := range nodes {
		if strings.HasPrefix(n, prefix) {
			out = append(out, n)
		}
	}
	return out
}
