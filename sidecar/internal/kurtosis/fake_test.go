package kurtosis

import (
	"context"
	"io"
)

type recordedRun struct {
	args []string
}

type fakeCLI struct {
	runs []recordedRun
	next func(args []string) (stdout, stderr string, err error)
}

func (f *fakeCLI) Run(_ context.Context, args []string, _ io.Reader, stdout, stderr io.Writer) error {
	f.runs = append(f.runs, recordedRun{args: append([]string(nil), args...)})
	if f.next == nil {
		return nil
	}
	out, errOut, err := f.next(args)
	_, _ = io.WriteString(stdout, out)
	_, _ = io.WriteString(stderr, errOut)
	return err
}
