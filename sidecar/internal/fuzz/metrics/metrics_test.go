package metrics

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegistry_ExposesFuzzCounters(t *testing.T) {
	r := New()
	r.TxsSubmitted.WithLabelValues("Payment", "valid").Inc()
	r.Divergences.WithLabelValues("tx_result").Inc()
	r.Crashes.WithLabelValues("goxrpl-0", "go_panic").Inc()

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body := readAll(t, resp.Body)

	for _, want := range []string{
		`fuzz_txs_submitted_total{mode="valid",tx_type="Payment"} 1`,
		`fuzz_divergences_total{layer="tx_result"} 1`,
		`fuzz_crashes_total{impl="go_panic",node="goxrpl-0"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing line %q in /metrics output", want)
		}
	}
}

func readAll(t *testing.T, r io.Reader) string {
	t.Helper()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
