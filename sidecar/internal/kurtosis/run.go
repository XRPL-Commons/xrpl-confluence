package kurtosis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
}

func (e *RunError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "kurtosis run: %v", e.Err)
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
		err := cli.Run(ctx, args, nil, &stdout, &stderr)
		if err == nil {
			return &RunResult{
				EnclaveID: opts.Enclave,
				Stdout:    stdout.String(),
				Stderr:    stderr.String(),
			}, nil
		}
		runErr := &RunError{
			Err:    err,
			Stdout: stdout.String(),
			Stderr: stderr.String(),
		}
		lastErr = runErr

		// Don't retry: ctx already cancelled, last attempt, or non-transient.
		if ctx.Err() != nil || attempt == maxAttempts || !runErr.IsTransient() {
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
