package chaos

import (
	"math/rand"
	"strings"
	"testing"
)

func TestExpandRecurring_FixedCount(t *testing.T) {
	spec := Recurring{
		Every:     100,
		Count:     3,
		StartStep: 50,
		Inner: rawEntry{
			Type:         "restart",
			Container:    "rippled-0",
			RecoverAfter: 10,
		},
	}
	env := ScheduleEnv{Nodes: []string{"rippled-0", "rippled-1"}, Seed: 1}
	out, err := ExpandRecurring(spec, env, rand.New(rand.NewSource(1)))
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("want 3 entries, got %d", len(out))
	}
	wantSteps := []int{50, 150, 250}
	for i, e := range out {
		if e.Step != wantSteps[i] {
			t.Errorf("entry %d: step %d, want %d", i, e.Step, wantSteps[i])
		}
		if e.Type != "restart" || e.Container != "rippled-0" || e.RecoverAfter != 10 {
			t.Errorf("entry %d: unexpected payload %+v", i, e)
		}
	}
}

func TestExpandRecurring_UntilStep(t *testing.T) {
	spec := Recurring{
		Every:     50,
		UntilStep: 175,
		StartStep: 0,
		Inner: rawEntry{
			Type:         "restart",
			Container:    "rippled-0",
			RecoverAfter: 5,
		},
	}
	out, err := ExpandRecurring(spec, ScheduleEnv{Nodes: []string{"rippled-0"}}, rand.New(rand.NewSource(1)))
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if len(out) != 4 {
		t.Fatalf("want 4 entries (0,50,100,150), got %d", len(out))
	}
}

func TestExpandRecurring_Wildcard(t *testing.T) {
	spec := Recurring{
		Every:     50,
		Count:     4,
		StartStep: 0,
		Inner: rawEntry{
			Type:         "restart",
			Container:    "rippled-*",
			RecoverAfter: 5,
		},
	}
	env := ScheduleEnv{Nodes: []string{"rippled-0", "rippled-1", "rippled-2", "goxrpl-0"}, Seed: 1}
	out, err := ExpandRecurring(spec, env, rand.New(rand.NewSource(1)))
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	for _, e := range out {
		if !strings.HasPrefix(e.Container, "rippled-") {
			t.Errorf("wildcard leaked non-rippled target: %q", e.Container)
		}
	}
	out2, _ := ExpandRecurring(spec, env, rand.New(rand.NewSource(1)))
	for i := range out {
		if out[i].Container != out2[i].Container {
			t.Errorf("non-deterministic: entry %d %q vs %q", i, out[i].Container, out2[i].Container)
		}
	}
}

func TestExpandRecurring_LatencyRange(t *testing.T) {
	spec := Recurring{
		Every:     100,
		Count:     5,
		StartStep: 0,
		Inner: rawEntry{
			Type:         "latency",
			Container:    "rippled-0",
			Iface:        "eth0",
			DelayMsMin:   50,
			DelayMsMax:   500,
			RecoverAfter: 60,
		},
	}
	env := ScheduleEnv{Nodes: []string{"rippled-0"}}
	out, err := ExpandRecurring(spec, env, rand.New(rand.NewSource(7)))
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	for _, e := range out {
		if e.DelayMs < 50 || e.DelayMs > 500 {
			t.Errorf("delay_ms %d outside [50,500]", e.DelayMs)
		}
	}
}

func TestExpandRecurring_ValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		spec Recurring
	}{
		{"every<=0", Recurring{Count: 1, Inner: rawEntry{Type: "restart"}}},
		{"no count or until", Recurring{Every: 10, Inner: rawEntry{Type: "restart"}}},
		{"missing inner type", Recurring{Every: 10, Count: 1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ExpandRecurring(c.spec, ScheduleEnv{}, rand.New(rand.NewSource(1)))
			if err == nil {
				t.Fatalf("want error, got nil")
			}
		})
	}
}
