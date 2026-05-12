package kurtosis

import (
	"context"
	"io"
	"os/exec"
)

// CLI is the abstraction over the `kurtosis` binary.
// Tests can substitute a fake; Exec shells out to the real binary.
type CLI interface {
	Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error
}

// Exec is the production CLI implementation.
type Exec struct {
	Binary string // default "kurtosis"
}

// NewExec returns an Exec that calls the real kurtosis binary.
func NewExec() *Exec { return &Exec{Binary: "kurtosis"} }

// Run executes the kurtosis binary with the given args, wiring IO as provided.
func (e *Exec) Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, e.Binary, args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}
