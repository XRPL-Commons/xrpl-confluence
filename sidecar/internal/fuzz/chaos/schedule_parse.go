package chaos

import (
	"encoding/json"
	"fmt"
	"math/rand"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// rawEntry mirrors the JSON wire format. All fields are optional;
// dispatch uses the `type` discriminator.
type rawEntry struct {
	Step         int    `json:"step"`
	RecoverAfter int    `json:"recover_after"`
	Type         string `json:"type"`
	Container    string `json:"container,omitempty"`
	From         string `json:"from,omitempty"`
	To           string `json:"to,omitempty"`
	Iface        string `json:"iface,omitempty"`
	DelayMs      int    `json:"delay_ms,omitempty"`
	DelayMsMin   int    `json:"delay_ms_min,omitempty"`
	DelayMsMax   int    `json:"delay_ms_max,omitempty"`
	Feature      string `json:"feature,omitempty"`
	Target       string `json:"target,omitempty"`

	// Recurring, when non-nil, indicates this entry is a recurring template.
	// During ParseSchedule it expands into many concrete rawEntries; the
	// outer Step/Container/etc. fields are ignored for recurring rows.
	Recurring *Recurring `json:"recurring,omitempty"`
}

// ParseSchedule converts the JSON wire format into a []ScheduleEntry.
// Unknown event types are rejected with a clear error so a typo doesn't
// silently degrade the chaos run. env supplies the node-name universe and
// run seed needed to expand recurring/wildcard entries deterministically.
func ParseSchedule(raw string, rt NetworkRuntime, env ScheduleEnv) ([]ScheduleEntry, error) {
	if raw == "" {
		return nil, nil
	}
	var entries []rawEntry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil, fmt.Errorf("schedule json: %w", err)
	}

	expanded := make([]rawEntry, 0, len(entries))
	for i, r := range entries {
		if r.Type == "recurring" || r.Recurring != nil {
			if r.Recurring == nil {
				return nil, fmt.Errorf("entry %d (recurring): recurring{} block required", i)
			}
			rng := rand.New(rand.NewSource(int64(env.Seed) ^ int64(i+1)))
			more, err := ExpandRecurring(*r.Recurring, env, rng)
			if err != nil {
				return nil, fmt.Errorf("entry %d: %w", i, err)
			}
			expanded = append(expanded, more...)
			continue
		}
		expanded = append(expanded, r)
	}

	out := make([]ScheduleEntry, 0, len(expanded))
	for i, r := range expanded {
		rng := rand.New(rand.NewSource(int64(env.Seed) ^ int64(i+1) ^ 0x5a5a5a5a))
		if err := resolveWildcards(&r, env, rng); err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
		var ev Event
		switch r.Type {
		case "restart":
			if r.Container == "" {
				return nil, fmt.Errorf("entry %d (restart): container is required", i)
			}
			ev = NewRestartEvent(rt, r.Container)
		case "latency":
			if r.Container == "" || r.Iface == "" || r.DelayMs <= 0 {
				return nil, fmt.Errorf("entry %d (latency): container/iface/delay_ms required", i)
			}
			ev = NewLatencyEvent(rt, r.Container, r.Iface, r.DelayMs)
		case "partition":
			if r.From == "" || r.To == "" {
				return nil, fmt.Errorf("entry %d (partition): from/to required", i)
			}
			ev = NewPartitionEvent(rt, r.From, r.To)
		case "amendment":
			if r.Feature == "" || r.Target == "" {
				return nil, fmt.Errorf("entry %d (amendment): feature/target required", i)
			}
			ev = NewAmendmentFlipEvent(rpcclient.New(r.Target), r.Feature)
		default:
			return nil, fmt.Errorf("entry %d: unknown event type %q", i, r.Type)
		}
		out = append(out, ScheduleEntry{
			TriggerStep:  r.Step,
			Apply:        ev,
			RecoverAfter: r.RecoverAfter,
		})
	}
	return out, nil
}
