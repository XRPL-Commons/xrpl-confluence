package chaos

import (
	"strings"
	"testing"
)

func TestParseSchedule_AllEventKinds(t *testing.T) {
	json := `[
	  {"step": 50, "recover_after": 25, "type": "restart", "container": "rippled-1"},
	  {"step": 100, "recover_after": 50, "type": "latency", "container": "rippled-0", "iface": "eth0", "delay_ms": 200},
	  {"step": 150, "recover_after": 30, "type": "partition", "from": "rippled-0", "to": "rippled-1"},
	  {"step": 200, "recover_after": 40, "type": "amendment", "feature": "FeatureFoo", "target": "http://rippled-0:5005"}
	]`
	rt := &fakeRuntime{}
	sched, err := ParseSchedule(json, rt, ScheduleEnv{})
	if err != nil {
		t.Fatal(err)
	}
	if len(sched) != 4 {
		t.Fatalf("len = %d, want 4", len(sched))
	}
	wantPrefixes := []string{"restart:", "latency:", "partition:", "amendment:"}
	for i, e := range sched {
		if !strings.HasPrefix(e.Apply.Name(), wantPrefixes[i]) {
			t.Errorf("entry %d name = %q, want prefix %q", i, e.Apply.Name(), wantPrefixes[i])
		}
	}
}

func TestParseSchedule_RejectsUnknownType(t *testing.T) {
	rt := &fakeRuntime{}
	_, err := ParseSchedule(`[{"step":1,"type":"bogus","container":"x"}]`, rt, ScheduleEnv{})
	if err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("err = %v, want contains 'bogus'", err)
	}
}

func TestParseSchedule_EmptyReturnsNil(t *testing.T) {
	sched, err := ParseSchedule("", nil, ScheduleEnv{})
	if err != nil {
		t.Fatal(err)
	}
	if sched != nil {
		t.Errorf("sched = %v, want nil", sched)
	}
}

func TestParseSchedule_RecurringExpands(t *testing.T) {
	raw := `[{
		"type": "recurring",
		"recurring": {
			"every": 100,
			"count": 3,
			"start_step": 50,
			"event": {"type": "restart", "container": "rippled-*", "recover_after": 5}
		}
	}]`
	env := ScheduleEnv{Nodes: []string{"rippled-0", "rippled-1"}, Seed: 42}
	entries, err := ParseSchedule(raw, &fakeRuntime{}, env)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("want 3 expanded entries, got %d", len(entries))
	}
	wantSteps := []int{50, 150, 250}
	for i, e := range entries {
		if e.TriggerStep != wantSteps[i] {
			t.Errorf("entry %d: step %d, want %d", i, e.TriggerStep, wantSteps[i])
		}
		if !strings.HasPrefix(e.Apply.Name(), "restart:rippled-") {
			t.Errorf("entry %d: name %q, want restart:rippled-*", i, e.Apply.Name())
		}
	}
}

func TestParseSchedule_PlainWildcardResolves(t *testing.T) {
	raw := `[{"step": 10, "type": "restart", "container": "goxrpl-*", "recover_after": 5}]`
	env := ScheduleEnv{Nodes: []string{"rippled-0", "goxrpl-0", "goxrpl-1"}, Seed: 7}
	entries, err := ParseSchedule(raw, &fakeRuntime{}, env)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1, got %d", len(entries))
	}
	if !strings.HasPrefix(entries[0].Apply.Name(), "restart:goxrpl-") {
		t.Errorf("name %q didn't resolve to a goxrpl-* target", entries[0].Apply.Name())
	}
}
