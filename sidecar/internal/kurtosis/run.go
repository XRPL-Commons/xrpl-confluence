package kurtosis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// RunOptions configures a kurtosis run invocation.
type RunOptions struct {
	Enclave       string
	PackageDir    string          // typically "."
	Args          json.RawMessage // already-compiled by scenario.Compile
	Verbose       bool
	TearDownFirst bool

	// MaxAttempts retries Run when the failure is classified as transient by
	// RunError.IsTransient (e.g. kurtosis package-upload stalls, empty bridge
	// IP on the API container). Each retry forcibly removes the half-booted
	// enclave first. Zero or negative is treated as 1 (no retry).
	//
	// Backoff is a fixed pause between attempts (default 5s). Non-transient
	// errors (e.g. Starlark consensus-readiness timeout) are returned
	// immediately without retry.
	MaxAttempts int
	RetryDelay  time.Duration

	// OnRetry, if set, is called before each retry attempt with the attempt
	// number (1-indexed for the upcoming attempt) and the prior error.
	OnRetry func(attempt int, prev error)

	// BootHangThreshold is the maximum time the kurtosis CLI may stay silent
	// (no bytes on stdout or stderr) before the watchdog cancels the run and
	// classifies the failure as transient so MaxAttempts retries it. A hung
	// kurtosis (0% CPU, no service progress) otherwise sits forever until the
	// outer ctx timeout fires, which on long-running soaks can be hours.
	// Zero disables the watchdog. Recommended default 90s on the up command.
	BootHangThreshold time.Duration

	// OnBootHang, if set, is called when the watchdog trips so callers can
	// surface a distinct human message (vs. an opaque "signal: killed").
	OnBootHang func(silenceFor time.Duration)
}

// RunResult holds the captured output of a kurtosis run.
type RunResult struct {
	EnclaveID string
	Stdout    string
	Stderr    string
}

// RunError wraps a non-zero kurtosis run exit. It preserves both stdout and
// stderr so callers can introspect (e.g. retry detection) without parsing the
// formatted error string.
//
// The kurtosis CLI prints Starlark "Caused by:" chains on stdout, not stderr,
// so both streams are kept on the error value.
type RunError struct {
	Err    error
	Stdout string
	Stderr string
	// BootHang is true when the kurtosis run was killed by the boot watchdog
	// because the CLI stayed silent past BootHangThreshold. Distinguished from
	// generic ctx-killed errors so callers can print a useful message and
	// retry classifies it as transient regardless of stderr content.
	BootHang bool
}

func (e *RunError) Error() string {
	var b strings.Builder
	if e.BootHang {
		fmt.Fprintf(&b, "kurtosis run: boot watchdog tripped (no output past threshold): %v", e.Err)
	} else {
		fmt.Fprintf(&b, "kurtosis run: %v", e.Err)
	}
	if s := lastLines(e.Stderr, 50); s != "" {
		fmt.Fprintf(&b, "\nstderr:\n%s", s)
	}
	if s := lastLines(e.Stdout, 50); s != "" {
		fmt.Fprintf(&b, "\nstdout (tail):\n%s", s)
	}
	return b.String()
}

func (e *RunError) Unwrap() error { return e.Err }

// IsTransient reports whether the kurtosis run failure looks like a
// kurtosis-engine transient (package-upload stall, empty bridge IP, signal
// killed by our own context timeout, etc.). Used by retry logic in callers.
func (e *RunError) IsTransient() bool {
	if e.BootHang {
		return true
	}
	merged := e.Stdout + "\n" + e.Stderr + "\n" + fmt.Sprint(e.Err)
	transientMarkers := []string{
		"signal: killed",
		"context deadline exceeded",
		"Error uploading package",
		"sending 'github.com/XRPL-Commons/xrpl-confluence'",
		"empty bridge network IP",
		"EOF",
	}
	for _, m := range transientMarkers {
		if strings.Contains(merged, m) {
			return true
		}
	}
	return false
}

// AsRunError returns the *RunError chained inside err, if any.
func AsRunError(err error) (*RunError, bool) {
	var re *RunError
	if errors.As(err, &re) {
		return re, true
	}
	return nil, false
}

// Run executes `kurtosis run --enclave <enclave> <packageDir> <argsJSON>`.
// Retries transient kurtosis-engine failures up to opts.MaxAttempts; see
// RunOptions docs and RunError.IsTransient.
func Run(ctx context.Context, cli CLI, opts RunOptions) (*RunResult, error) {
	maxAttempts := opts.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	retryDelay := opts.RetryDelay
	if retryDelay <= 0 {
		retryDelay = 5 * time.Second
	}

	args := []string{"run", "--enclave", opts.Enclave}
	if opts.Verbose {
		args = append(args, "-v")
	}
	args = append(args, opts.PackageDir)
	if len(opts.Args) > 0 {
		args = append(args, string(opts.Args))
	}

	var lastErr *RunError
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// On every attempt — including the first when TearDownFirst is set,
		// and every retry — wipe any pre-existing or half-booted enclave so
		// the package upload starts against a clean state.
		if opts.TearDownFirst || attempt > 1 {
			var buf bytes.Buffer
			_ = cli.Run(ctx, []string{"enclave", "rm", "-f", opts.Enclave}, nil, &buf, &buf)
		}

		var stdout, stderr bytes.Buffer
		err, bootHang, hangFor := runWithWatchdog(ctx, cli, args, &stdout, &stderr, opts.BootHangThreshold)
		if err == nil {
			return &RunResult{
				EnclaveID: opts.Enclave,
				Stdout:    stdout.String(),
				Stderr:    stderr.String(),
			}, nil
		}
		runErr := &RunError{
			Err:      err,
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			BootHang: bootHang,
		}
		if bootHang && opts.OnBootHang != nil {
			opts.OnBootHang(hangFor)
		}
		lastErr = runErr

		// Don't retry: ctx already cancelled, last attempt, or non-transient.
		// BootHang is always transient; the watchdog only fires when there's
		// nothing useful to inspect (kurtosis stuck at 0% CPU before any
		// service progress), so a fresh attempt is genuinely worth trying.
		if (ctx.Err() != nil && !bootHang) || attempt == maxAttempts || !runErr.IsTransient() {
			return nil, runErr
		}

		if opts.OnRetry != nil {
			opts.OnRetry(attempt+1, runErr)
		}
		select {
		case <-ctx.Done():
			return nil, runErr
		case <-time.After(retryDelay):
		}
	}
	// Unreachable: loop always returns inside.
	return nil, lastErr
}

// runWithWatchdog wraps cli.Run so a kurtosis CLI that goes completely silent
// past threshold is killed and surfaced as a BootHang failure. Without the
// watchdog, ~1-in-3 boots hang at 0% CPU before any service comes up and the
// operator has to pkill kurtosis by hand. Returns (err, bootHang, silenceFor).
//
// Threshold <= 0 disables the watchdog entirely — the call becomes equivalent
// to a direct cli.Run. Activity is sampled on every byte the CLI emits to
// stdout or stderr; an actively-uploading or actively-launching kurtosis is
// chatty enough that any silence longer than ~30s is structural.
func runWithWatchdog(ctx context.Context, cli CLI, args []string, stdout, stderr *bytes.Buffer, threshold time.Duration) (error, bool, time.Duration) {
	if threshold <= 0 {
		return cli.Run(ctx, args, nil, stdout, stderr), false, 0
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	tracker := &activityTracker{}
	tracker.touch()

	wOut := &activityWriter{w: stdout, t: tracker}
	wErr := &activityWriter{w: stderr, t: tracker}

	var (
		bootHang bool
		silence  time.Duration
		watchMu  sync.Mutex
	)

	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(threshold / 4)
		if threshold/4 <= 0 {
			ticker = time.NewTicker(threshold)
		}
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-runCtx.Done():
				return
			case <-ticker.C:
				if d := tracker.silenceFor(); d >= threshold {
					watchMu.Lock()
					bootHang = true
					silence = d
					watchMu.Unlock()
					cancel()
					return
				}
			}
		}
	}()

	err := cli.Run(runCtx, args, nil, wOut, wErr)
	close(done)

	watchMu.Lock()
	hung, dur := bootHang, silence
	watchMu.Unlock()
	if hung && err == nil {
		// CLI somehow finished cleanly after watchdog cancelled — treat as success.
		return nil, false, 0
	}
	return err, hung, dur
}

// activityTracker holds a monotonic timestamp (unix nanos) updated on every
// byte the wrapped CLI writes. Touched atomically so the watchdog goroutine
// can sample it without locking.
type activityTracker struct {
	lastNS atomic.Int64
}

func (a *activityTracker) touch() {
	a.lastNS.Store(time.Now().UnixNano())
}

func (a *activityTracker) silenceFor() time.Duration {
	last := a.lastNS.Load()
	if last == 0 {
		return 0
	}
	return time.Since(time.Unix(0, last))
}

type activityWriter struct {
	w io.Writer
	t *activityTracker
}

func (w *activityWriter) Write(p []byte) (int, error) {
	if len(p) > 0 {
		w.t.touch()
	}
	return w.w.Write(p)
}

// lastLines returns the last n lines of s, trimmed.
func lastLines(s string, n int) string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}
