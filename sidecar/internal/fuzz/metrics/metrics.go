// Package metrics exposes the fuzz_* Prometheus surface called out in the
// xrpl-confluence fuzzer design.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry wraps a private *prometheus.Registry so the fuzzer's metrics
// don't leak into any default registries.
type Registry struct {
	reg *prometheus.Registry

	TxsSubmitted   *prometheus.CounterVec   // labels: tx_type, mode (valid|mutated|random)
	TxsApplied     *prometheus.CounterVec   // labels: tx_type, result (e.g. tesSUCCESS)
	Divergences    *prometheus.CounterVec   // labels: layer (state_hash|tx_result|metadata|invariant|crash)
	Crashes        *prometheus.CounterVec   // labels: node, impl
	AccountsActive   prometheus.Gauge
	CorpusSize       prometheus.Gauge
	UniqueSignatures prometheus.Gauge
	CurrentSeed      prometheus.Gauge
	OracleLatency  *prometheus.HistogramVec // labels: layer
	CloseDuration  prometheus.Histogram
}

// New constructs and registers all collectors on a fresh registry.
func New() *Registry {
	r := &Registry{reg: prometheus.NewRegistry()}

	r.TxsSubmitted = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "fuzz_txs_submitted_total"},
		[]string{"tx_type", "mode"},
	)
	r.TxsApplied = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "fuzz_txs_applied_total"},
		[]string{"tx_type", "result"},
	)
	r.Divergences = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "fuzz_divergences_total"},
		[]string{"layer"},
	)
	r.Crashes = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "fuzz_crashes_total"},
		[]string{"node", "impl"},
	)
	r.AccountsActive = prometheus.NewGauge(prometheus.GaugeOpts{Name: "fuzz_accounts_active"})
	r.CorpusSize = prometheus.NewGauge(prometheus.GaugeOpts{Name: "fuzz_corpus_size"})
	r.UniqueSignatures = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "fuzz_unique_signatures",
		Help: "Distinct divergence signatures observed in the current run.",
	})
	r.CurrentSeed = prometheus.NewGauge(prometheus.GaugeOpts{Name: "fuzz_current_seed"})
	r.OracleLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Name: "fuzz_oracle_latency_seconds", Buckets: prometheus.DefBuckets},
		[]string{"layer"},
	)
	r.CloseDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{Name: "fuzz_close_duration_seconds", Buckets: prometheus.DefBuckets},
	)

	for _, c := range []prometheus.Collector{
		r.TxsSubmitted, r.TxsApplied, r.Divergences, r.Crashes,
		r.AccountsActive, r.CorpusSize, r.UniqueSignatures, r.CurrentSeed,
		r.OracleLatency, r.CloseDuration,
	} {
		r.reg.MustRegister(c)
	}
	return r
}

// Handler returns an http.Handler serving the fuzz_* metrics in
// Prometheus text exposition format.
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.reg, promhttp.HandlerOpts{})
}
