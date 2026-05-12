package main

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

// runLogsCmd injects logsDeps into the root and runs args.
func runLogsCmd(t *testing.T, deps *logsDeps, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	outBuf, errBuf := &bytes.Buffer{}, &bytes.Buffer{}
	root := newRootCmd()
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	replaceSubCmd(root, newLogsCmdWith(deps))
	root.SetArgs(args)
	err = root.ExecuteContext(context.Background())
	return outBuf.String(), errBuf.String(), err
}

const cannedLogs = "2026-05-12T14:00:00Z INFO  rippled-0 started\n2026-05-12T14:00:01Z WARN  rippled-0 peer connected\n"

func fakeCLIForLogs(enclave, node string) *fakeCLI {
	lsOut := "UUID\tName\tStatus\n" + "abc123\t" + enclave + "\tRUNNING\n"
	return &fakeCLI{
		next: func(args []string) (string, string, error) {
			if len(args) >= 2 && args[0] == "enclave" && args[1] == "ls" {
				return lsOut, "", nil
			}
			if len(args) >= 2 && args[0] == "service" && args[1] == "logs" {
				// Write canned lines to stdout writer — the fakeCLI.Run method
				// receives the writer and writes our canned output.
				return cannedLogs, "", nil
			}
			return "", "", nil
		},
	}
}

func TestLogs_HappyPath(t *testing.T) {
	withDiscoveryDir(t)

	cli := fakeCLIForLogs("my-enclave", "rippled-0")
	deps := &logsDeps{cli: cli}

	stdout, _, err := runLogsCmd(t, deps, "logs", "--node", "rippled-0", "--enclave", "my-enclave")
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	if !strings.Contains(stdout, "rippled-0") {
		t.Errorf("expected node name in output, got %q", stdout)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) < 2 {
		t.Errorf("expected at least 2 log lines, got %d", len(lines))
	}

	// Verify kurtosis was called with "service logs <enclave> <node>".
	var gotLogsCall bool
	for _, run := range cli.runs {
		if len(run) >= 4 && run[0] == "service" && run[1] == "logs" && run[2] == "my-enclave" && run[3] == "rippled-0" {
			gotLogsCall = true
			break
		}
	}
	if !gotLogsCall {
		t.Errorf("expected service logs call with enclave+node; got: %v", cli.runs)
	}
}

func TestLogs_Follow(t *testing.T) {
	withDiscoveryDir(t)

	cli := &fakeCLI{
		next: func(args []string) (string, string, error) {
			return cannedLogs, "", nil
		},
	}
	deps := &logsDeps{cli: cli}

	_, _, _ = runLogsCmd(t, deps, "logs", "--node", "rippled-0", "--enclave", "my-enclave", "--follow")

	// Verify --follow was passed to kurtosis.
	var gotFollow bool
	for _, run := range cli.runs {
		if len(run) >= 2 && run[0] == "service" && run[1] == "logs" {
			for _, a := range run {
				if a == "--follow" {
					gotFollow = true
				}
			}
		}
	}
	if !gotFollow {
		t.Errorf("expected --follow in service logs args; got: %v", cli.runs)
	}
}

func TestLogs_GrepFilter(t *testing.T) {
	withDiscoveryDir(t)

	multiLineLogs := "INFO  started\nWARN  peer connected\nINFO  ledger closed\n"
	cli := &fakeCLI{
		next: func(args []string) (stdout, stderr string, err error) {
			if len(args) >= 2 && args[0] == "service" && args[1] == "logs" {
				return multiLineLogs, "", nil
			}
			return "", "", nil
		},
	}
	deps := &logsDeps{cli: cli}

	stdout, _, err := runLogsCmd(t, deps, "logs", "--node", "rippled-0", "--enclave", "my-enclave", "--grep", "WARN")
	if err != nil {
		t.Fatalf("logs --grep: %v", err)
	}
	if !strings.Contains(stdout, "peer connected") {
		t.Errorf("expected WARN line in output, got %q", stdout)
	}
	if strings.Contains(stdout, "started") || strings.Contains(stdout, "ledger closed") {
		t.Errorf("non-matching lines leaked through: %q", stdout)
	}
}

func TestLogs_MissingNode(t *testing.T) {
	withDiscoveryDir(t)

	deps := &logsDeps{cli: &fakeCLI{
		next: func(args []string) (string, string, error) {
			return "", "", nil
		},
	}}

	// Write canned output to /dev/null — we only care about the error.
	outBuf, errBuf := &bytes.Buffer{}, &bytes.Buffer{}
	root := newRootCmd()
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	replaceSubCmd(root, newLogsCmdWith(deps))
	root.SetArgs([]string{"logs", "--enclave", "my-enclave"})
	err := root.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected error when --node not provided")
	}
	if !strings.Contains(err.Error(), "--node") {
		t.Errorf("error should mention --node, got: %v", err)
	}
	_ = io.Discard
}
