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

	TxsSubmitted     *prometheus.CounterVec   // labels: tx_type, mode (valid|mutated|random)
	TxsApplied       *prometheus.CounterVec   // labels: tx_type, result (e.g. tesSUCCESS)
	TxsFailed        *prometheus.CounterVec   // labels: tx_type, result (e.g. tecXXX, temXXX, rpc_error)
	Divergences      *prometheus.CounterVec   // labels: layer (state_hash|tx_result|metadata|invariant|crash|consensus_stall|peer_drop)
	Crashes          *prometheus.CounterVec   // labels: node, impl
	AccountsActive   prometheus.Gauge
	CorpusSize       prometheus.Gauge
	UniqueSignatures prometheus.Gauge
	CurrentSeed      prometheus.Gauge
	OracleLatency    *prometheus.HistogramVec // labels: layer
	CloseDuration    prometheus.Histogram

	// Network-side gauges sampled by the liveness monitor.
	NodeValidatedSeq      *prometheus.GaugeVec // labels: node
	NodePeerCount         *prometheus.GaugeVec // labels: node
	NodeLastCloseConverge *prometheus.GaugeVec // labels: node
	// Wall-clock seconds since any node last advanced its validated_ledger.
	// 0 means progressing. Rises monotonically during a stall.
	NetworkStallSeconds prometheus.Gauge
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
	r.TxsFailed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fuzz_txs_failed_total",
			Help: "Submissions that did not reach tesSUCCESS/terQUEUED, labelled by engine code (tec*/tem*/tef*/temBAD*/...) or rpc_error.",
		},
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
	r.NodeValidatedSeq = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "fuzz_node_validated_seq", Help: "Validated ledger sequence per node (sampled by the liveness monitor)."},
		[]string{"node"},
	)
	r.NodePeerCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "fuzz_node_peer_count", Help: "Number of peers each node reports (sampled by the liveness monitor)."},
		[]string{"node"},
	)
	r.NodeLastCloseConverge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "fuzz_node_last_close_converge_seconds", Help: "Time the most recent consensus round took on each node."},
		[]string{"node"},
	)
	r.NetworkStallSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{Name: "fuzz_network_stall_seconds", Help: "Wall-clock seconds since the latest validated_ledger advance across any node. 0 while the network is progressing."},
	)

	for _, c := range []prometheus.Collector{
		r.TxsSubmitted, r.TxsApplied, r.TxsFailed, r.Divergences, r.Crashes,
		r.AccountsActive, r.CorpusSize, r.UniqueSignatures, r.CurrentSeed,
		r.OracleLatency, r.CloseDuration,
		r.NodeValidatedSeq, r.NodePeerCount, r.NodeLastCloseConverge,
		r.NetworkStallSeconds,
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
