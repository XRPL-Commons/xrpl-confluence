package runners

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/chaos"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/fuzz/corpus"
)

// ChaosConfig wraps SoakConfig with a deterministic chaos schedule.
type ChaosConfig struct {
	SoakConfig
	Schedule []chaos.ScheduleEntry
}

// ChaosRun runs the soak loop with a chaos scheduler attached. Returns
// (soak stats, chaos stats, err).
func ChaosRun(ctx context.Context, cfg ChaosConfig) (*Stats, *chaos.Stats, error) {
	sched := chaos.NewChaosScheduler(cfg.Schedule)

	rec := corpus.NewRecorder(cfg.CorpusDir, cfg.Seed)
	sched.OnAudit = func(a chaos.AuditEntry) {
		blob, _ := json.Marshal(a)
		_, _ = rec.RecordDivergence(&corpus.Divergence{
			Kind:        "chaos",
			Description: fmt.Sprintf("%s/%s at step %d", a.Event, a.Phase, a.Step),
			Details:     map[string]any{"audit": json.RawMessage(blob)},
		})
		if a.Error != "" {
			cfg.Alerter.Maybe("chaos:"+a.Event, fmt.Sprintf("chaos event errored: %s/%s step %d: %s", a.Event, a.Phase, a.Step, a.Error))
		}
		if cfg.Metrics != nil {
			cfg.Metrics.Divergences.WithLabelValues("chaos").Inc()
		}
	}

	cfg.SoakConfig.OnPeriodic = func(step int) {
		sched.Step(ctx, step)
		if cfg.Metrics != nil {
			if entries, err := os.ReadDir(filepath.Join(cfg.CorpusDir, "divergences")); err == nil {
				cfg.Metrics.CorpusSize.Set(float64(len(entries)))
			}
		}
	}

	stats, err := SoakRun(ctx, cfg.SoakConfig)
	chaosStats := sched.Stats()
	return stats, &chaosStats, err
}
