package runners

import (
	"context"
	"testing"
	"time"
)

// TestSoakRun_StopsOnContextCancel boots a soak run with a tight context and
// verifies it returns cleanly with stats covering the work it managed to do.
func TestSoakRun_StopsOnContextCancel(t *testing.T) {
	tmp := t.TempDir()
	cfg := SoakConfig{
		Config: Config{
			NodeURLs:  []string{"http://stub-a", "http://stub-b"},
			SubmitURL: "http://stub-a",
			Seed:      7,
			AccountN:  2,
			CorpusDir: tmp,
			SkipFund:  true,
			SkipSetup: true,
		},
		TxRate:      1,   // 1 tx/s
		RotateEvery: 100, // accounts rotate after 100 successes
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	stats, err := SoakRun(ctx, cfg)
	if err != nil && err != context.DeadlineExceeded {
		t.Fatalf("SoakRun: %v", err)
	}
	if stats == nil {
		t.Fatal("nil stats")
	}
}
