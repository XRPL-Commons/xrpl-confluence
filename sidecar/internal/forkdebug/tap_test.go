package forkdebug

import (
	"bytes"
	"strings"
	"testing"
)

// TestParseLine_CtAvalanche pins parsing of the close-time
// avalanche event — the line that surfaced the seq=18 tie-break
// bug. The fields the formatter renders MUST round-trip.
func TestParseLine_CtAvalanche(t *testing.T) {
	line := `2026/05/11 08:09:56 INFO close-time avalanche t=consensus event=ct-avalanche seq=18 mode=proposing converge_pct=39 avalanche_state=mid needed_weight=50 thresh_vote=3 thresh_consensus=4 participants=6 have_consensus=false consensus_ct_xrpl=831802190 our_pos_ct_xrpl=831802194 our_pos_seq=0 votes="831802200=3 831802190=3"`

	ev, ok := ParseLine(line)
	if !ok {
		t.Fatal("expected ct-avalanche line to parse, got skip")
	}
	if ev.Kind != "ct-avalanche" {
		t.Errorf("Kind = %q, want ct-avalanche", ev.Kind)
	}
	if ev.Seq != 18 {
		t.Errorf("Seq = %d, want 18", ev.Seq)
	}
	if ev.Mode != "proposing" {
		t.Errorf("Mode = %q, want proposing", ev.Mode)
	}
	if got := ev.Fields["have_consensus"]; got != "false" {
		t.Errorf("have_consensus = %q, want false", got)
	}
	if got := ev.Fields["votes"]; got != "831802200=3 831802190=3" {
		t.Errorf("votes = %q, want quoted-string round-trip", got)
	}
	if got := ev.Fields["avalanche_state"]; got != "mid" {
		t.Errorf("avalanche_state = %q, want mid", got)
	}

	formatted := FormatTapEvent(ev)
	for _, must := range []string{"seq=18", "ct-vote", "state=mid", "have_consensus=false", "votes=831802200=3 831802190=3"} {
		if !strings.Contains(formatted, must) {
			t.Errorf("formatted output missing %q\nfull: %s", must, formatted)
		}
	}
}

// TestParseLine_ModeChange pins the bespoke mode-change line shape:
// goxrpl logs these without an event= field, so the parser falls
// back to substring matching. The formatter must still produce a
// useful summary.
func TestParseLine_ModeChange(t *testing.T) {
	line := `2026/05/11 08:15:20 INFO Consensus mode changed component=consensus-adaptor from=observing to=wrongLedger`

	ev, ok := ParseLine(line)
	if !ok {
		t.Fatal("mode-change line must parse")
	}
	if ev.Kind != "mode-change" {
		t.Errorf("Kind = %q, want mode-change", ev.Kind)
	}
	if ev.Fields["from"] != "observing" || ev.Fields["to"] != "wrongLedger" {
		t.Errorf("from/to = %q/%q, want observing/wrongLedger",
			ev.Fields["from"], ev.Fields["to"])
	}
	if got := FormatTapEvent(ev); got != "MODE observing -> wrongLedger" {
		t.Errorf("format = %q", got)
	}
}

// TestParseLine_ValidateGateSkip pins recognition of the
// "skipped validation" decision — the signal that meant goxrpl
// stopped contributing to quorum. False-negatives on this kind
// would mask exactly the failure mode we're hunting.
func TestParseLine_ValidateGateSkip(t *testing.T) {
	line := `2026/05/11 08:15:16 INFO validation gate t=consensus event=validate-gate seq=98 hash=4b660e3c3025269c result=success is_validator=true consensus_fail=false wrong_lcl=true can_validate_seq=true our_last_validated_seq=20 mode=wrongLedger decision=skip:wrong-lcl`

	ev, ok := ParseLine(line)
	if !ok {
		t.Fatal("validate-gate skip line must parse")
	}
	if ev.Kind != "validate-gate-skip" {
		t.Errorf("Kind = %q, want validate-gate-skip", ev.Kind)
	}
	if ev.Seq != 98 {
		t.Errorf("Seq = %d, want 98", ev.Seq)
	}
	if got := FormatTapEvent(ev); !strings.Contains(got, "SKIP-VAL") || !strings.Contains(got, "skip:wrong-lcl") {
		t.Errorf("format missing SKIP markers: %s", got)
	}
}

// TestParseLine_LedgerBuilt confirms that the per-round summary
// line yields the seq, mode, hash, tx_count, and result fields the
// formatter relies on.
func TestParseLine_LedgerBuilt(t *testing.T) {
	line := `2026/05/11 08:15:28 INFO ledger built t=consensus event=ledger-built seq=102 hash=ac2013a7c8e64827 parent_seq=101 parent_hash=50f04a86abfc5ddb parent_ct_xrpl=831802522 close_time_xrpl=831802530 close_time_correct=true resolution_s=10 tx_set=0000000000000000 tx_count=0 result=success mode=wrongLedger`

	ev, ok := ParseLine(line)
	if !ok {
		t.Fatal("ledger-built line must parse")
	}
	if ev.Kind != "ledger-built" {
		t.Errorf("Kind = %q, want ledger-built", ev.Kind)
	}
	if got := FormatTapEvent(ev); !strings.Contains(got, "seq=102") || !strings.Contains(got, "result=success") {
		t.Errorf("format missing key fields: %s", got)
	}
}

// TestParseLine_NoiseSkipped guards against false positives. Lines
// that aren't structured consensus events must NOT yield TapEvents,
// or callers piping in mixed log streams will get noise.
func TestParseLine_NoiseSkipped(t *testing.T) {
	for _, line := range []string{
		"",
		"plain text without keys",
		"2026/05/11 INFO some other system: foo=bar baz=qux",       // has kvs but no event=
		"random line without equals or recognized substrings",
	} {
		if ev, ok := ParseLine(line); ok {
			t.Errorf("noise line wrongly parsed as %q: %s", ev.Kind, line)
		}
	}
}

// TestTap_StreamingEndToEnd pipes a small log mix into Tap and
// asserts only the recognized events come through, in order.
// Non-matching lines must be silently dropped so a long log tail
// stays readable.
func TestTap_StreamingEndToEnd(t *testing.T) {
	input := strings.Join([]string{
		`unrelated startup line`,
		`2026/05/11 08:09:54 INFO validation emitted t=consensus event=validate-emit seq=17 hash=c8e7c22c24024fce full=true sign_time_xrpl=831802194`,
		`some intermediate log noise with key=value`,
		`2026/05/11 08:09:55 INFO our initial position t=consensus-build event=our-position round_seq=18 prev=c8e7c22c24024fce`,
		`2026/05/11 08:09:56 INFO close-time avalanche t=consensus event=ct-avalanche seq=18 mode=proposing converge_pct=39 avalanche_state=mid needed_weight=50 thresh_vote=3 thresh_consensus=4 participants=6 have_consensus=false consensus_ct_xrpl=831802190 our_pos_ct_xrpl=831802194 our_pos_seq=0 votes="831802200=3 831802190=3"`,
		`2026/05/11 08:15:20 INFO Consensus mode changed component=consensus-adaptor from=observing to=wrongLedger`,
	}, "\n")

	var out bytes.Buffer
	if err := Tap(strings.NewReader(input), &out); err != nil {
		t.Fatalf("Tap: %v", err)
	}

	got := out.String()
	wantInOrder := []string{
		"EMIT-VAL",                        // validate-emit seq=17
		"seq=18 proposing ct-vote",        // ct-avalanche
		"MODE observing -> wrongLedger",   // mode-change
	}

	pos := 0
	for _, marker := range wantInOrder {
		idx := strings.Index(got[pos:], marker)
		if idx < 0 {
			t.Errorf("missing or out-of-order marker %q\noutput:\n%s", marker, got)
			break
		}
		pos += idx + len(marker)
	}

	// "our-position" event has no recognized kind in classifyEvent,
	// so it must be dropped — guards against the parser silently
	// accepting unknown events into the stream.
	if strings.Contains(got, "our-position") {
		t.Errorf("unrecognized event slipped through: %s", got)
	}
}
