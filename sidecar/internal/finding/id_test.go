package finding

import (
	"strings"
	"testing"
)

func TestIDPrefixesAndUniqueness(t *testing.T) {
	cases := []struct {
		name   string
		fn     func() string
		prefix string
	}{
		{"finding", NewFindingID, "fnd_"},
		{"run", NewRunID, "run_"},
		{"reproducer", NewReproducerID, "rpr_"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, b := tc.fn(), tc.fn()
			if !strings.HasPrefix(a, tc.prefix) {
				t.Fatalf("missing prefix %q: %q", tc.prefix, a)
			}
			if a == b {
				t.Fatalf("ids collided: %q", a)
			}
			// ULID part is 26 chars (Crockford base32).
			if len(a) != len(tc.prefix)+26 {
				t.Fatalf("unexpected length %d: %q", len(a), a)
			}
		})
	}
}
