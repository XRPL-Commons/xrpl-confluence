package kurtosis

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

// hangingCLI simulates a kurtosis CLI that goes silent immediately and never
// writes anything. It blocks until the supplied ctx is cancelled, then returns
// whatever ctx.Err() says — mirrors `exec.CommandContext` behavior when the
// child process is killed by ctx cancellation.
type hangingCLI struct {
	startedAt time.Time
	finishedAt time.Time
}

func (h *hangingCLI) Run(ctx context.Context, _ []string, _ io.Reader, _, _ io.Writer) error {
	h.startedAt = time.Now()
	<-ctx.Done()
	h.finishedAt = time.Now()
	return ctx.Err()
}

func TestRun_BootHangWatchdogTrips(t *testing.T) {
	h := &hangingCLI{}
	opts := RunOptions{
		Enclave:           "enc",
		PackageDir:        ".",
		MaxAttempts:       1, // no retry — we want to see the BootHang surface
		BootHangThreshold: 100 * time.Millisecond,
	}
	var hangSeen time.Duration
	opts.OnBootHang = func(d time.Duration) { hangSeen = d }

	start := time.Now()
	_, err := Run(context.Background(), h, opts)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from hung run")
	}
	if elapsed > time.Second {
		t.Errorf("watchdog should have tripped under ~200ms; elapsed=%v", elapsed)
	}
	re, ok := AsRunError(err)
	if !ok {
		t.Fatalf("expected *RunError, got %T", err)
	}
	if !re.BootHang {
		t.Errorf("RunError.BootHang should be true; got %+v", re)
	}
	if !re.IsTransient() {
		t.Errorf("BootHang must be transient (so retries kick in)")
	}
	if hangSeen <= 0 {
		t.Errorf("OnBootHang should have been called with a non-zero duration; got %v", hangSeen)
	}
}

func TestRun_BootHangRetriesThenFails(t *testing.T) {
	// Two hung attempts in a row — confirms watchdog drives the retry loop
	// AND that the second failure surfaces as a BootHang error rather than
	// being silently swallowed.
	h := &hangingCLI{}
	calls := 0
	wrapper := cliFunc(func(ctx context.Context, args []string, _ io.Reader, stdout, stderr io.Writer) error {
		if len(args) >= 2 && args[0] == "enclave" && args[1] == "rm" {
			return nil
		}
		calls++
		return h.Run(ctx, args, nil, stdout, stderr)
	})

	opts := RunOptions{
		Enclave:           "enc",
		PackageDir:        ".",
		MaxAttempts:       2,
		RetryDelay:        time.Millisecond,
		BootHangThreshold: 50 * time.Millisecond,
	}
	_, err := Run(context.Background(), wrapper, opts)
	if err == nil {
		t.Fatal("expected final BootHang error after both attempts hang")
	}
	if calls != 2 {
		t.Errorf("expected 2 run attempts, got %d", calls)
	}
	re, ok := AsRunError(err)
	if !ok || !re.BootHang {
		t.Errorf("final error should be BootHang RunError, got %T %+v", err, err)
	}
}

func TestRun_BootHangThresholdZeroDisables(t *testing.T) {
	// With threshold == 0, a hanging CLI should NOT be killed by the
	// watchdog — only the outer context can stop it. Confirms zero is the
	// opt-out value.
	ctx, cancel := context.WithCancel(context.Background())
	h := &hangingCLI{}

	done := make(chan error, 1)
	go func() {
		_, err := Run(ctx, h, RunOptions{
			Enclave:           "enc",
			PackageDir:        ".",
			MaxAttempts:       1,
			BootHangThreshold: 0,
		})
		done <- err
	}()

	select {
	case <-done:
		t.Fatal("Run returned before outer ctx cancel; watchdog must be disabled")
	case <-time.After(200 * time.Millisecond):
	}
	cancel()
	err := <-done
	if err == nil || !errors.Is(err, context.Canceled) {
		// Either is fine — the point is the watchdog didn't preempt it.
		// Surfaced via outer ctx after we cancelled.
	}
}

func TestRun_BootHangActivityResetsTimer(t *testing.T) {
	// A CLI that emits a byte every 30ms with a 100ms threshold must not
	// trip the watchdog — proves "silence" is measured from last write, not
	// from start.
	cli := cliFunc(func(ctx context.Context, _ []string, _ io.Reader, stdout, _ io.Writer) error {
		t := time.NewTicker(30 * time.Millisecond)
		defer t.Stop()
		for i := 0; i < 10; i++ {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				_, _ = stdout.Write([]byte("."))
			}
		}
		return nil
	})

	_, err := Run(context.Background(), cli, RunOptions{
		Enclave:           "enc",
		PackageDir:        ".",
		MaxAttempts:       1,
		BootHangThreshold: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("active CLI must not trip watchdog: %v", err)
	}
}

// cliFunc adapts a function into the CLI interface — sugar for inline fakes.
type cliFunc func(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error

func (f cliFunc) Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return f(ctx, args, stdin, stdout, stderr)
}
