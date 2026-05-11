package forkdebug

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// TapEvent is a single parsed line from goxrpl's structured
// consensus log. We surface the few events that mattered during
// the issue #401 investigation; other lines are ignored.
type TapEvent struct {
	// Kind discriminates the event family. Values:
	//   "ct-avalanche"        — close-time vote distribution snapshot
	//   "accept-ct"           — close-time decision committed for a round
	//   "ledger-built"        — ledger header materialized from consensus
	//   "validate-emit"       — validation broadcast
	//   "validate-gate-skip"  — validation withheld (and reason)
	//   "mode-change"         — consensus operating mode transition
	//   "wrong-lcl"           — entered wrongLedger because peer LCL not acquirable
	Kind string `json:"kind"`
	// Seq is the round/ledger seq if the event line carries one.
	// Zero when the event is not seq-scoped.
	Seq uint32 `json:"seq,omitempty"`
	// Mode is the consensus mode reported on the line, when present.
	Mode string `json:"mode,omitempty"`
	// Fields holds all key=value pairs verbatim. Useful for kinds
	// where structured access mattered less than seeing every field.
	Fields map[string]string `json:"fields"`
}

// FormatTapEvent renders a TapEvent on a single line. The shape is
// intentionally compact: in a stuck network you'll see hundreds of
// these per round and the failure pattern is in the cadence, not
// any single line.
func FormatTapEvent(e TapEvent) string {
	switch e.Kind {
	case "ct-avalanche":
		return fmt.Sprintf("seq=%d %s ct-vote: state=%s convergence=%s%% have_consensus=%s votes=%s our_pos_seq=%s",
			e.Seq, e.Mode,
			e.Fields["avalanche_state"],
			e.Fields["converge_pct"],
			e.Fields["have_consensus"],
			e.Fields["votes"],
			e.Fields["our_pos_seq"],
		)
	case "accept-ct":
		return fmt.Sprintf("seq=%d %s ACCEPT-CT branch=%s eff_ct=%s have_ct_consensus=%s",
			e.Seq, e.Mode,
			e.Fields["ct_branch"],
			e.Fields["eff_ct_xrpl"],
			e.Fields["have_ct_consensus"],
		)
	case "ledger-built":
		return fmt.Sprintf("seq=%d %s BUILT hash=%s tx_count=%s result=%s",
			e.Seq, e.Mode,
			e.Fields["hash"],
			e.Fields["tx_count"],
			e.Fields["result"],
		)
	case "validate-emit":
		return fmt.Sprintf("seq=%d EMIT-VAL hash=%s full=%s",
			e.Seq, e.Fields["hash"], e.Fields["full"],
		)
	case "validate-gate-skip":
		return fmt.Sprintf("seq=%d SKIP-VAL hash=%s reason=%s mode=%s",
			e.Seq, e.Fields["hash"], e.Fields["decision"], e.Mode,
		)
	case "mode-change":
		return fmt.Sprintf("MODE %s -> %s", e.Fields["from"], e.Fields["to"])
	case "wrong-lcl":
		return fmt.Sprintf("WRONG-LCL hash=%s", e.Fields["hash"])
	default:
		// Stable key ordering for unknown kinds so log diffs are
		// deterministic.
		keys := make([]string, 0, len(e.Fields))
		for k := range e.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var sb strings.Builder
		fmt.Fprintf(&sb, "%s", e.Kind)
		for _, k := range keys {
			fmt.Fprintf(&sb, " %s=%s", k, e.Fields[k])
		}
		return sb.String()
	}
}

// kvLineRe pulls out the key=value pairs from a structured log
// line. goxrpl uses log/slog text format: bare keys followed by
// `=` and either a quoted or bareword value.
//
// We tolerate both `key=value` and `key="quoted value"`.
var kvLineRe = regexp.MustCompile(`(\w+)=(?:"((?:[^"\\]|\\.)*)"|(\S+))`)

// classifyEvent maps the `event=` field (when present) to a TapEvent
// Kind. Falls back to the `msg=` content for slog-styled log lines
// that don't use the `event=` convention.
func classifyEvent(fields map[string]string) string {
	if ev, ok := fields["event"]; ok {
		switch ev {
		case "ct-avalanche":
			return "ct-avalanche"
		case "accept-ct":
			return "accept-ct"
		case "ledger-built":
			return "ledger-built"
		case "validate-emit":
			return "validate-emit"
		case "validate-gate":
			if fields["decision"] != "" && strings.HasPrefix(fields["decision"], "skip:") {
				return "validate-gate-skip"
			}
			return ""
		case "wrong-lcl":
			return "wrong-lcl"
		}
	}
	if msg, ok := fields["msg"]; ok {
		if strings.Contains(msg, "Consensus mode changed") {
			return "mode-change"
		}
	}
	return ""
}

// extractFromTo reads a "from=X to=Y" pair into the Fields map for
// mode-change lines, which goxrpl logs in a different shape than
// the structured `event=` family.
func extractFromTo(line string) (from, to string) {
	// "Consensus mode changed component=consensus-adaptor from=X to=Y"
	if i := strings.Index(line, "from="); i >= 0 {
		rest := line[i+len("from="):]
		if sp := strings.IndexAny(rest, " \t"); sp >= 0 {
			from = rest[:sp]
		} else {
			from = rest
		}
	}
	if i := strings.Index(line, "to="); i >= 0 {
		rest := line[i+len("to="):]
		if sp := strings.IndexAny(rest, " \t"); sp >= 0 {
			to = rest[:sp]
		} else {
			to = rest
		}
	}
	return from, to
}

// ParseLine parses one line of goxrpl text-format log output and
// returns a TapEvent if the line corresponds to a recognized
// consensus signal, or (TapEvent{}, false) otherwise.
//
// The parser is intentionally permissive: keys it doesn't know are
// preserved in Fields. Unknown event kinds return a TapEvent with
// the raw event-name as Kind so callers can extend the formatter
// without touching the parser.
func ParseLine(line string) (TapEvent, bool) {
	if !strings.Contains(line, "=") {
		return TapEvent{}, false
	}

	fields := make(map[string]string)
	for _, m := range kvLineRe.FindAllStringSubmatch(line, -1) {
		key := m[1]
		val := m[2]
		if val == "" {
			val = m[3]
		}
		fields[key] = val
	}

	// Mode-change lines don't carry event= but follow a known prefix.
	if strings.Contains(line, "Consensus mode changed") {
		from, to := extractFromTo(line)
		return TapEvent{
			Kind:   "mode-change",
			Fields: map[string]string{"from": from, "to": to},
		}, true
	}

	kind := classifyEvent(fields)
	if kind == "" {
		return TapEvent{}, false
	}

	ev := TapEvent{
		Kind:   kind,
		Mode:   fields["mode"],
		Fields: fields,
	}
	if seqStr, ok := fields["seq"]; ok {
		if n, err := strconv.ParseUint(seqStr, 10, 32); err == nil {
			ev.Seq = uint32(n)
		}
	}
	return ev, true
}

// Tap reads goxrpl log output line-by-line from r, parses each line
// for consensus events, and emits the formatted summary on w. Stops
// when r closes (EOF) or returns an error other than EOF.
//
// Designed to wrap `docker logs -f goxrpl-0`.
func Tap(r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	// goxrpl debug log lines can be long (full hex dumps in
	// validation-rejected diagnostics). Bump the buffer up front
	// rather than failing mid-stream on a perfectly recoverable
	// long line.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		ev, ok := ParseLine(line)
		if !ok {
			continue
		}
		if _, err := fmt.Fprintln(w, FormatTapEvent(ev)); err != nil {
			return fmt.Errorf("tap write: %w", err)
		}
	}
	return scanner.Err()
}
