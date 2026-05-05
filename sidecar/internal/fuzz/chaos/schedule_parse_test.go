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
	sched, err := ParseSchedule(json, rt)
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
	_, err := ParseSchedule(`[{"step":1,"type":"bogus","container":"x"}]`, rt)
	if err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("err = %v, want contains 'bogus'", err)
	}
}

func TestParseSchedule_EmptyReturnsNil(t *testing.T) {
	sched, err := ParseSchedule("", nil)
	if err != nil {
		t.Fatal(err)
	}
	if sched != nil {
		t.Errorf("sched = %v, want nil", sched)
	}
}
