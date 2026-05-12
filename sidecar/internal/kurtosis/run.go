package kurtosis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

// RunOptions configures a kurtosis run invocation.
type RunOptions struct {
	Enclave       string
	PackageDir    string          // typically "."
	Args          json.RawMessage // already-compiled by scenario.Compile
	Verbose       bool
	TearDownFirst bool
}

// RunResult holds the captured output of a kurtosis run.
type RunResult struct {
	EnclaveID string
	Stdout    string
	Stderr    string
}

// Run executes `kurtosis run --enclave <enclave> <packageDir> <argsJSON>`.
func Run(ctx context.Context, cli CLI, opts RunOptions) (*RunResult, error) {
	if opts.TearDownFirst {
		var buf bytes.Buffer
		_ = cli.Run(ctx, []string{"enclave", "rm", "-f", opts.Enclave}, nil, &buf, &buf)
	}

	args := []string{"run", "--enclave", opts.Enclave}
	if opts.Verbose {
		args = append(args, "-v")
	}
	args = append(args, opts.PackageDir)
	if len(opts.Args) > 0 {
		args = append(args, string(opts.Args))
	}

	var stdout, stderr bytes.Buffer
	err := cli.Run(ctx, args, nil, &stdout, &stderr)
	if err != nil {
		return nil, fmt.Errorf("kurtosis run: %w\n%s", err, stderr.String())
	}
	return &RunResult{
		EnclaveID: opts.Enclave,
		Stdout:    stdout.String(),
		Stderr:    stderr.String(),
	}, nil
}
