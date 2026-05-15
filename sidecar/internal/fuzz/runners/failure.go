package runners

import (
	"fmt"
	"log"
	"strings"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/alert"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/metrics"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// classifyFailure maps a (result, error) pair from a submit call to a
// stable label suitable for metrics + run-log "result" field.
//
// Order: RPC error wins over engine result (no result on transport failure).
// For RPC errors we extract the rippled error code if present
// ("highFee", "noCurrent", ...) and otherwise fall back to "rpc_error".
// For engine failures we return the raw EngineResult code (e.g. "tecUNFUNDED_OFFER").
func classifyFailure(res *rpcclient.SubmitResult, err error) string {
	if err != nil {
		msg := err.Error()
		for _, code := range []string{
			"highFee", "noCurrent", "notReady", "tooBusy",
			"invalidTransaction", "internal", "amendmentBlocked",
		} {
			if strings.Contains(msg, code) {
				return code
			}
		}
		return "rpc_error"
	}
	if res != nil && res.EngineResult != "" {
		return res.EngineResult
	}
	return "unknown"
}

// recordFailure increments the failure metric, appends a run-log entry so
// the failed tx is reproducible / shrinkable, and emits a sampled log line.
//
// txLog and m may be nil — both updates are best-effort.
func recordFailure(
	m *metrics.Registry,
	txLog *corpus.RunLog,
	logSampleN int,
	logSeq *int64,
	step int,
	txType string,
	fields map[string]any,
	secret string,
	res *rpcclient.SubmitResult,
	submitErr error,
) {
	code := classifyFailure(res, submitErr)

	if m != nil {
		m.TxsFailed.WithLabelValues(txType, code).Inc()
	}
	if txLog != nil {
		// TxHash is empty: the tx didn't land. Result holds the engine code
		// or RPC error class so reproduce/shrink can target this submission.
		_ = txLog.Append(&corpus.RunLogEntry{
			Step:   step,
			TxType: txType,
			Fields: fields,
			Secret: secret,
			Result: code,
		})
	}

	if logSampleN > 0 && logSeq != nil {
		*logSeq++
		if *logSeq%int64(logSampleN) == 1 {
			if submitErr != nil {
				log.Printf("submit %s failed: %s — %v", txType, code, submitErr)
			} else if res != nil {
				log.Printf("submit %s failed: %s (%s)", txType, code, res.EngineResultMessage)
			} else {
				log.Printf("submit %s failed: %s", txType, code)
			}
		}
	}
}

// recordSetupFailure persists a setup-phase abort as a divergence finding
// and fires the alerter, so downstream tooling parsing /output/corpus
// sees "fuzzer aborted at setup" rather than an empty corpus.
//
// `phase` should identify the failing step ("fund", "setup_state",
// "discover_amendments", ...). `runMode` propagates through to the
// finding so soak / realtime / replay / shrink share one corpus schema.
func recordSetupFailure(
	rec *corpus.Recorder,
	m *metrics.Registry,
	alerter *alert.Webhook,
	runMode string,
	phase string,
	err error,
) {
	if err == nil {
		return
	}
	desc := fmt.Sprintf("setup aborted in %s: %v", phase, err)
	log.Printf("%s: %s", runMode, desc)

	d := &corpus.Divergence{
		Kind:        "setup_failure",
		Description: desc,
		Details: map[string]any{
			"mode":  runMode,
			"phase": phase,
			"error": err.Error(),
		},
	}
	if rec != nil {
		_, _ = rec.RecordDivergence(d)
	}
	if m != nil {
		m.Divergences.WithLabelValues("setup_failure").Inc()
	}
	if alerter != nil {
		alerter.Maybe(corpus.Signature(d).Key(),
			fmt.Sprintf("[setup_failure] %s: %s", runMode, desc))
	}
}
