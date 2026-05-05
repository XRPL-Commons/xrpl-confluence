package runners

import (
	"context"
	"testing"
	"time"
)

func TestChaosRun_StopsOnContextCancel(t *testing.T) {
	tmp := t.TempDir()
	cfg := ChaosConfig{
		SoakConfig: SoakConfig{
			Config: Config{
				NodeURLs:  []string{"http://stub-a", "http://stub-b"},
				SubmitURL: "http://stub-a",
				Seed:      11,
				AccountN:  2,
				CorpusDir: tmp,
				SkipFund:  true,
				SkipSetup: true,
			},
			TxRate:      1,
			RotateEvery: 100,
		},
		Schedule: nil,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	stats, _, err := ChaosRun(ctx, cfg)
	if err != nil && err != context.DeadlineExceeded {
		t.Fatalf("ChaosRun: %v", err)
	}
	if stats == nil {
		t.Fatal("nil soak stats")
	}
}
