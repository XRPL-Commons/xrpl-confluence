package kurtosis

import (
	"bytes"
	"context"
	"testing"
)

func TestServiceLogs_WithFollow(t *testing.T) {
	f := &fakeCLI{
		next: func(args []string) (string, string, error) {
			return "log line 1\nlog line 2\n", "", nil
		},
	}
	var buf bytes.Buffer
	if err := ServiceLogs(context.Background(), f, "enc", "svc", true, &buf); err != nil {
		t.Fatalf("ServiceLogs: %v", err)
	}
	if len(f.runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(f.runs))
	}
	args := f.runs[0].args
	// Expect: service logs --follow enc svc
	if args[0] != "service" || args[1] != "logs" || args[2] != "--follow" || args[3] != "enc" || args[4] != "svc" {
		t.Errorf("unexpected args: %v", args)
	}
	if buf.String() != "log line 1\nlog line 2\n" {
		t.Errorf("unexpected output: %q", buf.String())
	}
}

func TestServiceLogs_WithoutFollow(t *testing.T) {
	f := &fakeCLI{}
	var buf bytes.Buffer
	if err := ServiceLogs(context.Background(), f, "enc", "svc", false, &buf); err != nil {
		t.Fatalf("ServiceLogs: %v", err)
	}
	args := f.runs[0].args
	// Expect: service logs enc svc (no --follow)
	if args[0] != "service" || args[1] != "logs" || args[2] != "enc" || args[3] != "svc" {
		t.Errorf("unexpected args: %v", args)
	}
}
