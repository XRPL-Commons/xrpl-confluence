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
