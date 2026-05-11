package forkdebug

import (
	"strings"
	"testing"
	"time"
)

// TestNewStallDetector_Validation rejects misconfigured CLI input.
func TestNewStallDetector_Validation(t *testing.T) {
	if _, err := NewStallDetector(nil); err == nil {
		t.Error("nil nodes must error")
	}
	if _, err := NewStallDetector([]Node{{Name: "", URL: "http://x"}}); err == nil {
		t.Error("missing name must error")
	}
	if _, err := NewStallDetector([]Node{{Name: "a", URL: ""}}); err == nil {
		t.Error("missing URL must error")
	}
}

// TestFormatStallResult_StalledLayout pins the wire shape of the
// stalled report. CI consumers parse line 1 for the verdict, so
// the "STALLED" / "OK" prefix must be stable.
func TestFormatStallResult_StalledLayout(t *testing.T) {
	r := &StallResult{
		WindowSeconds:  30,
		PollInterval:   3 * time.Second,
		Stalled:        true,
		PerNodeAdvance: map[string]int{"goxrpl-0": 0, "rippled-0": 0},
		First: []StallSample{
			{Node: "goxrpl-0", ValidatedSeq: 5, ServerState: "proposing"},
			{Node: "rippled-0", ValidatedSeq: 5, ServerState: "proposing"},
		},
		Last: []StallSample{
			{Node: "goxrpl-0", ValidatedSeq: 5, ServerState: "proposing"},
			{Node: "rippled-0", ValidatedSeq: 5, ServerState: "proposing"},
		},
	}
	out := FormatStallResult(r)
	if !strings.HasPrefix(out, "STALLED") {
		t.Errorf("verdict line must start with STALLED, got: %s", strings.SplitN(out, "\n", 2)[0])
	}
	for _, must := range []string{"30s window", "goxrpl-0", "rippled-0", "+0", "⏸"} {
		if !strings.Contains(out, must) {
			t.Errorf("missing %q in:\n%s", must, out)
		}
	}
}

// TestFormatStallResult_OKLayout pins the OK case: at least one
// node advanced. The verdict must say OK and per-node deltas show
// the advance with a +N sign.
func TestFormatStallResult_OKLayout(t *testing.T) {
	r := &StallResult{
		WindowSeconds:  30,
		PerNodeAdvance: map[string]int{"goxrpl-0": 12, "rippled-0": 12},
		First: []StallSample{
			{Node: "goxrpl-0", ValidatedSeq: 100, ServerState: "proposing"},
			{Node: "rippled-0", ValidatedSeq: 100, ServerState: "proposing"},
		},
		Last: []StallSample{
			{Node: "goxrpl-0", ValidatedSeq: 112, ServerState: "proposing"},
			{Node: "rippled-0", ValidatedSeq: 112, ServerState: "proposing"},
		},
	}
	out := FormatStallResult(r)
	if !strings.HasPrefix(out, "OK") {
		t.Errorf("verdict line must start with OK, got: %s", strings.SplitN(out, "\n", 2)[0])
	}
	if !strings.Contains(out, "+12") {
		t.Errorf("missing advance delta in:\n%s", out)
	}
}

// TestFormatStallResult_RegressionFlag pins the visual marker for
// a NEGATIVE delta. Catching ledger regressions (validated_seq
// going down) was a real failure mode in the closedLedger no-
// regress investigation; the report must not silently swallow it.
func TestFormatStallResult_RegressionFlag(t *testing.T) {
	r := &StallResult{
		WindowSeconds:  30,
		PerNodeAdvance: map[string]int{"goxrpl-0": -3},
		First: []StallSample{
			{Node: "goxrpl-0", ValidatedSeq: 50, ServerState: "proposing"},
		},
		Last: []StallSample{
			{Node: "goxrpl-0", ValidatedSeq: 47, ServerState: "proposing"},
		},
	}
	out := FormatStallResult(r)
	if !strings.Contains(out, "-3") {
		t.Errorf("negative delta -3 missing in:\n%s", out)
	}
	if !strings.Contains(out, "↓") {
		t.Errorf("regression marker ↓ missing in:\n%s", out)
	}
}
