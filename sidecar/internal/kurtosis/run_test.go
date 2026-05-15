package kurtosis

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestRun_HappyPath(t *testing.T) {
	f := &fakeCLI{
		next: func(args []string) (string, string, error) {
			return "enclave started\n", "", nil
		},
	}
	opts := RunOptions{
		Enclave:    "my-enclave",
		PackageDir: ".",
		Args:       json.RawMessage(`{"nodes":2}`),
	}
	res, err := Run(context.Background(), f, opts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.EnclaveID != "my-enclave" {
		t.Errorf("EnclaveID: got %q, want %q", res.EnclaveID, "my-enclave")
	}
	if res.Stdout != "enclave started\n" {
		t.Errorf("Stdout: got %q", res.Stdout)
	}
	// Verify args: run --enclave my-enclave . {"nodes":2}
	if len(f.runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(f.runs))
	}
	args := f.runs[0].args
	if args[0] != "run" || args[1] != "--enclave" || args[2] != "my-enclave" || args[3] != "." || args[4] != `{"nodes":2}` {
		t.Errorf("unexpected args: %v", args)
	}
}

func TestRun_NonZeroExitReturnsError(t *testing.T) {
	f := &fakeCLI{
		next: func(args []string) (string, string, error) {
			return "", "something went wrong", errors.New("exit status 1")
		},
	}
	_, err := Run(context.Background(), f, RunOptions{Enclave: "enc", PackageDir: "."})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "something went wrong") {
		t.Errorf("error should contain stderr text, got: %v", err)
	}
	if re, ok := AsRunError(err); !ok {
		t.Errorf("expected *RunError, got %T", err)
	} else if re.Stderr == "" {
		t.Errorf("RunError.Stderr should be populated")
	}
}

func TestRun_NonZeroExitIncludesStdout(t *testing.T) {
	// kurtosis CLI prints Starlark "Caused by:" chains on stdout, so the error
	// must surface stdout even when stderr is empty.
	f := &fakeCLI{
		next: func(args []string) (string, string, error) {
			return "Error: An error occurred running command 'run'\n  Caused by: noCurrent ledger\n", "", errors.New("exit status 1")
		},
	}
	_, err := Run(context.Background(), f, RunOptions{Enclave: "enc", PackageDir: "."})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Caused by: noCurrent ledger") {
		t.Errorf("error should surface stdout Caused-by chain, got: %v", err)
	}
}

func TestRun_RetriesTransient(t *testing.T) {
	calls := 0
	f := &fakeCLI{
		next: func(args []string) (string, string, error) {
			if args[0] == "enclave" && args[1] == "rm" {
				// teardown calls don't count
				return "", "", nil
			}
			calls++
			if calls < 3 {
				return "Error uploading package 'github.com/X/Y'\nCaused by: EOF\n", "", errors.New("exit status 1")
			}
			return "Created enclave\n", "", nil
		},
	}
	opts := RunOptions{
		Enclave:     "enc",
		PackageDir:  ".",
		MaxAttempts: 3,
		RetryDelay:  1, // tiny — test should be fast
	}
	res, err := Run(context.Background(), f, opts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 run attempts, got %d", calls)
	}
	if res == nil || res.EnclaveID != "enc" {
		t.Errorf("expected success result, got %+v", res)
	}
	// Each retry must be preceded by an enclave rm to clean up half-booted state.
	rmCount := 0
	for _, r := range f.runs {
		if len(r.args) >= 2 && r.args[0] == "enclave" && r.args[1] == "rm" {
			rmCount++
		}
	}
	if rmCount < 2 {
		t.Errorf("expected enclave rm between retries, got %d rm calls", rmCount)
	}
}

func TestRun_DoesNotRetryStarlarkError(t *testing.T) {
	// A non-transient Starlark error (e.g. consensus readiness timeout) must
	// fail fast — retrying won't fix it.
	calls := 0
	f := &fakeCLI{
		next: func(args []string) (string, string, error) {
			if args[0] == "enclave" && args[1] == "rm" {
				return "", "", nil
			}
			calls++
			return "Error: An error occurred running command 'run'\nCaused by: rippled-1 readiness timeout\n", "", errors.New("exit status 1")
		},
	}
	_, err := Run(context.Background(), f, RunOptions{
		Enclave:     "enc",
		PackageDir:  ".",
		MaxAttempts: 5,
		RetryDelay:  1,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("non-transient error must not retry; got %d attempts", calls)
	}
}

func TestRunError_IsTransient(t *testing.T) {
	cases := []struct {
		name   string
		err    *RunError
		expect bool
	}{
		{"signal killed", &RunError{Err: errors.New("signal: killed")}, true},
		{"upload eof", &RunError{Stdout: "Error uploading package 'github.com/X/Y'\nCaused by: EOF"}, true},
		{"empty bridge ip", &RunError{Stderr: "empty bridge network IP address for API container"}, true},
		{"starlark error", &RunError{Stdout: "Caused by: rippled-1 readiness timeout"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.err.IsTransient(); got != c.expect {
				t.Errorf("IsTransient = %v, want %v", got, c.expect)
			}
		})
	}
}

func TestRun_TearDownFirst(t *testing.T) {
	f := &fakeCLI{}
	opts := RunOptions{
		Enclave:       "enc",
		PackageDir:    ".",
		TearDownFirst: true,
	}
	_, err := Run(context.Background(), f, opts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(f.runs) != 2 {
		t.Fatalf("expected 2 runs (rm + run), got %d", len(f.runs))
	}
	if f.runs[0].args[0] != "enclave" || f.runs[0].args[1] != "rm" {
		t.Errorf("first call should be enclave rm, got %v", f.runs[0].args)
	}
}
