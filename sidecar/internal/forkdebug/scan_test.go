package forkdebug

import (
	"strings"
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/oracle"
)

// TestNewScanner_Validation rejects the misconfigurations a CLI
// user is most likely to make: too few nodes, missing name, missing
// URL. Surfacing these at construction beats producing a confusing
// "ledger 1 not found" cascade later.
func TestNewScanner_Validation(t *testing.T) {
	if _, err := NewScanner(nil); err == nil {
		t.Error("nil nodes must error")
	}
	if _, err := NewScanner([]Node{{Name: "only", URL: "http://x"}}); err == nil {
		t.Error("single node must error (need >= 2 to compare)")
	}
	if _, err := NewScanner([]Node{
		{Name: "", URL: "http://x"},
		{Name: "b", URL: "http://y"},
	}); err == nil {
		t.Error("missing name must error")
	}
	if _, err := NewScanner([]Node{
		{Name: "a", URL: ""},
		{Name: "b", URL: "http://y"},
	}); err == nil {
		t.Error("missing URL must error")
	}
}

// TestFormatScanResult_NoFork pins the all-clear summary path:
// when no divergence was found, the formatter must say so plainly,
// include the swept range, and report the LastAgreed seq so the
// caller knows how far the network is consistent.
func TestFormatScanResult_NoFork(t *testing.T) {
	r := &ScanResult{
		FromSeq:    1,
		ToSeq:      40,
		Compared:   40,
		LastAgreed: &oracle.LedgerComparison{Sequence: 40, Agreed: true},
	}
	out := FormatScanResult(r)
	for _, must := range []string{"Scanned seq=[1..40]", "40 ledgers compared", "No fork detected", "Last agreed seq: 40"} {
		if !strings.Contains(out, must) {
			t.Errorf("missing %q in:\n%s", must, out)
		}
	}
}

// TestFormatScanResult_ForkLayout pins the fork report layout:
// must say "FIRST FORK at seq=N", show every node's hashes in
// stable (name-sorted) order, list the divergent fields, and
// surface the immediate ancestor for the seq=N → seq=N+1 framing
// callers will use to choose an isolate-source ledger.
func TestFormatScanResult_ForkLayout(t *testing.T) {
	r := &ScanResult{
		FromSeq: 1, ToSeq: 50, Compared: 18,
		LastAgreed: &oracle.LedgerComparison{Sequence: 17, Agreed: true},
		FirstFork: &oracle.LedgerComparison{
			Sequence: 18,
			Agreed:   false,
			NodeHashes: []oracle.NodeHash{
				{Name: "rippled-0", LedgerHash: "AABBCC0011223344", AccountHash: "11", TransactionHash: "22"},
				{Name: "goxrpl-0", LedgerHash: "FFEEDD9988776655", AccountHash: "33", TransactionHash: "22"},
			},
			Divergences: []oracle.Divergence{
				{Field: "ledger_hash", NodeA: "rippled-0", HashA: "AABBCC0011223344",
					NodeB: "goxrpl-0", HashB: "FFEEDD9988776655"},
				{Field: "account_hash", NodeA: "rippled-0", HashA: "11",
					NodeB: "goxrpl-0", HashB: "33"},
			},
		},
	}

	out := FormatScanResult(r)

	for _, must := range []string{
		"FIRST FORK at seq=18",
		"last-agreed seq=17",
		"transitioning 17 → 18",
		"goxrpl-0",
		"rippled-0",
		"divergent fields:",
		"ledger_hash",
		"account_hash",
	} {
		if !strings.Contains(out, must) {
			t.Errorf("missing %q in:\n%s", must, out)
		}
	}

	// Per-node table must be sorted by node name. goxrpl-0 < rippled-0
	// alphabetically, so goxrpl-0's row appears first.
	gIdx := strings.Index(out, "goxrpl-0")
	rIdx := strings.Index(out, "rippled-0")
	if gIdx < 0 || rIdx < 0 || gIdx >= rIdx {
		t.Errorf("nodes not in sorted order: goxrpl-0 idx=%d rippled-0 idx=%d", gIdx, rIdx)
	}
}

// TestIsRealFork pins the false-positive guard: an availability
// gap (some nodes erroring) is NOT a fork, even though the oracle
// marks Agreed=false on any error. A fork requires ≥2 responding
// nodes pointing at different hashes.
//
// Without this distinction `forkdebug scan --from 1` against a
// freshly-started 5-node network always reports a phantom fork at
// seq=1 because rippled prunes/hides the genesis-adjacent rows.
func TestIsRealFork(t *testing.T) {
	cases := []struct {
		name string
		cmp  *oracle.LedgerComparison
		want bool
	}{
		{"nil", nil, false},
		{"agreed", &oracle.LedgerComparison{Agreed: true}, false},
		{
			name: "all errors no hashes",
			cmp: &oracle.LedgerComparison{
				Agreed: false,
				Errors: []string{"a: not found", "b: not found"},
			},
			want: false,
		},
		{
			name: "two responded same hash, third errored — not a fork",
			cmp: &oracle.LedgerComparison{
				Agreed: false, // oracle flags this because of the error
				NodeHashes: []oracle.NodeHash{
					{Name: "a", LedgerHash: "ABCD"},
					{Name: "b", LedgerHash: "ABCD"},
				},
				Errors: []string{"c: not found"},
				// Divergences is empty — the two responders agreed.
			},
			want: false,
		},
		{
			name: "real divergence between two responders",
			cmp: &oracle.LedgerComparison{
				Agreed: false,
				NodeHashes: []oracle.NodeHash{
					{Name: "a", LedgerHash: "AAAA"},
					{Name: "b", LedgerHash: "BBBB"},
				},
				Divergences: []oracle.Divergence{
					{Field: "ledger_hash", NodeA: "a", HashA: "AAAA", NodeB: "b", HashB: "BBBB"},
				},
			},
			want: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isRealFork(c.cmp); got != c.want {
				t.Errorf("isRealFork = %v, want %v", got, c.want)
			}
		})
	}
}

// TestShortHex truncates long hashes but preserves short ones
// untouched, and renders empty strings as "-" so the table column
// still aligns when a node failed to return a hash.
func TestShortHex(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "-"},
		{"abcd", "abcd"},
		{"AABBCC0011223344", "AABBCC0011223344"},
		{"AABBCC00112233445566", "AABBCC0011223344..."},
	}
	for _, c := range cases {
		if got := shortHex(c.in); got != c.want {
			t.Errorf("shortHex(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
