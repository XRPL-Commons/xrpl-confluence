// Package crash inspects container exits and log tails to identify
// process-level failures (panics, asserts, segfaults) that the oracle
// layers cannot see by themselves.
package crash

import "strings"

// Classification names a recognised failure pattern in the tail of a
// container's log. An empty Kind means no marker was found.
type Classification struct {
	Kind       string // "go_panic", "rippled_fatal", "sigsegv", "sigabrt", ""
	MarkerLine int    // index into the excerpt where the marker matched
}

var markers = []struct {
	kind     string
	patterns []string
}{
	{"go_panic", []string{"panic:", "fatal error:"}},
	{"rippled_fatal", []string{"FATAL:", "ASSERT", "assertion failed"}},
	{"sigsegv", []string{"SIGSEGV", "signal SIGSEGV", "segmentation violation"}},
	{"sigabrt", []string{"SIGABRT", "signal SIGABRT", "Aborted (core dumped)"}},
}

// Classify inspects an excerpt (one element per log line, oldest first)
// and returns the first matching classification.
func Classify(excerpt []string) Classification {
	for i, line := range excerpt {
		for _, m := range markers {
			for _, pat := range m.patterns {
				if strings.Contains(line, pat) {
					return Classification{Kind: m.kind, MarkerLine: i}
				}
			}
		}
	}
	return Classification{}
}
