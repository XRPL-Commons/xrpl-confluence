package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeDockerExec records calls and simulates docker ps / docker cp.
type fakeDockerExec struct {
	containers  []string // returned by Ps
	copyCalls   []cpCall // recorded CopyFromContainer invocations
	copyErr     error
	copyPayload map[string]string // dest dir → filename to create
}

type cpCall struct {
	Container string
	Src       string
	Dest      string
}

func (f *fakeDockerExec) Ps(_ context.Context) ([]string, error) {
	return f.containers, nil
}

func (f *fakeDockerExec) CopyFromContainer(_ context.Context, container, src, dest string) error {
	f.copyCalls = append(f.copyCalls, cpCall{Container: container, Src: src, Dest: dest})
	if f.copyErr != nil {
		return f.copyErr
	}
	// Simulate file creation so countFiles works.
	if f.copyPayload != nil {
		for dir, name := range f.copyPayload {
			if strings.HasSuffix(dest, dir) || dest == dir {
				_ = os.WriteFile(filepath.Join(dest, name), []byte("data"), 0o644)
			}
		}
	}
	return nil
}

// fakePullCLI stubs kurtosis CLI for pull tests.
type fakePullCLI struct {
	// service UUID returned by InspectService for a given service name
	uuids map[string]string
	// list of service names returned by InspectEnclave
	services []string
}

func (f *fakePullCLI) Run(_ context.Context, args []string, _ io.Reader, stdout, _ io.Writer) error {
	// We only handle the sub-commands needed by pull.
	if len(args) >= 3 && args[0] == "service" && args[1] == "inspect" {
		svcName := args[3]
		uuid := f.uuids[svcName]
		fmt.Fprintf(stdout, "UUID: %s\n", uuid)
		fmt.Fprintf(stdout, "IP Address: 10.0.0.1\n")
		return nil
	}
	if len(args) >= 2 && args[0] == "enclave" && args[1] == "inspect" {
		fmt.Fprintln(stdout, "== Services ==")
		fmt.Fprintln(stdout, "Name  UUID  Status")
		for _, s := range f.services {
			fmt.Fprintf(stdout, "%s  abc  RUNNING\n", s)
		}
		return nil
	}
	return nil
}

func TestPull_Findings_HappyPath(t *testing.T) {
	tmp := t.TempDir()

	docker := &fakeDockerExec{
		containers: []string{"confluence-control--uuid-ctrl-1"},
		copyPayload: map[string]string{
			"findings": "finding-001.json",
		},
	}
	cli := &fakePullCLI{
		uuids: map[string]string{"confluence-control": "uuid-ctrl-1"},
	}

	outBuf := &bytes.Buffer{}
	root := newRootCmd()
	root.SetOut(outBuf)
	root.SetErr(&bytes.Buffer{})
	_ = root.PersistentFlags().Set("enclave", "myenclave")

	cmd := newPullCmd()
	root.AddCommand(cmd)
	cmd.SetOut(outBuf)
	_ = cmd.Flags().Set("findings", "true")
	_ = cmd.Flags().Set("corpus", "false")
	_ = cmd.Flags().Set("dest", tmp)
	cmd.SetContext(context.Background())

	if err := runPullWith(cmd, cli, docker); err != nil {
		t.Fatalf("runPullWith: %v", err)
	}

	// Assert docker cp was called with the right args.
	if len(docker.copyCalls) != 1 {
		t.Fatalf("expected 1 copy call, got %d", len(docker.copyCalls))
	}
	call := docker.copyCalls[0]
	if !strings.HasPrefix(call.Container, "confluence-control--uuid-ctrl-1") {
		t.Errorf("unexpected container: %q", call.Container)
	}
	if call.Src != "/var/confluence/findings/." {
		t.Errorf("unexpected src: %q", call.Src)
	}
	if !strings.HasSuffix(call.Dest, "findings") {
		t.Errorf("unexpected dest: %q", call.Dest)
	}

	// Output contains "Pulled findings".
	if !strings.Contains(outBuf.String(), "Pulled findings") {
		t.Errorf("expected 'Pulled findings' in output, got: %q", outBuf.String())
	}
}

func TestPull_Corpus_AutoDetectFuzzService(t *testing.T) {
	tmp := t.TempDir()

	docker := &fakeDockerExec{
		containers: []string{"fuzz-soak--uuid-fuzz-1"},
		copyPayload: map[string]string{
			"corpus": "seed-001",
		},
	}
	cli := &fakePullCLI{
		uuids:    map[string]string{"fuzz-soak": "uuid-fuzz-1"},
		services: []string{"confluence-control", "fuzz-soak"},
	}

	outBuf := &bytes.Buffer{}
	cmd := newPullCmd()
	cmd.SetOut(outBuf)
	cmd.SetErr(&bytes.Buffer{})
	_ = cmd.Flags().Set("findings", "false")
	_ = cmd.Flags().Set("corpus", "true")
	_ = cmd.Flags().Set("dest", tmp)
	cmd.SetContext(context.Background())

	root2 := newRootCmd()
	root2.AddCommand(cmd)
	_ = root2.PersistentFlags().Set("enclave", "myenclave")

	if err := runPullWith(cmd, cli, docker); err != nil {
		t.Fatalf("runPullWith: %v", err)
	}

	if len(docker.copyCalls) != 1 {
		t.Fatalf("expected 1 copy call, got %d", len(docker.copyCalls))
	}
	call := docker.copyCalls[0]
	if !strings.HasPrefix(call.Container, "fuzz-soak--uuid-fuzz-1") {
		t.Errorf("unexpected container: %q", call.Container)
	}
	if call.Src != "/output/corpus/." {
		t.Errorf("unexpected src: %q", call.Src)
	}
}

func TestPull_JSON_Output(t *testing.T) {
	tmp := t.TempDir()

	docker := &fakeDockerExec{
		containers:  []string{"confluence-control--uuid-ctrl-1"},
		copyPayload: map[string]string{"findings": "f1.json"},
	}
	cli := &fakePullCLI{
		uuids: map[string]string{"confluence-control": "uuid-ctrl-1"},
	}

	outBuf := &bytes.Buffer{}
	root := newRootCmd()
	root.SetOut(outBuf)
	root.SetErr(&bytes.Buffer{})
	_ = root.PersistentFlags().Set("enclave", "myenclave")
	_ = root.PersistentFlags().Set("json", "true")

	cmd := newPullCmd()
	root.AddCommand(cmd)
	cmd.SetOut(outBuf)
	_ = cmd.Flags().Set("findings", "true")
	_ = cmd.Flags().Set("corpus", "false")
	_ = cmd.Flags().Set("dest", tmp)
	cmd.SetContext(context.Background())

	if err := runPullWith(cmd, cli, docker); err != nil {
		t.Fatalf("runPullWith: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(outBuf.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v (got %q)", err, outBuf.String())
	}
	if got["enclave_id"] != "myenclave" {
		t.Errorf("enclave_id: got %v", got["enclave_id"])
	}
	copied, ok := got["copied"].([]any)
	if !ok || len(copied) != 1 {
		t.Errorf("copied: expected 1 entry, got %v", got["copied"])
	}
}

func TestPull_NothingEnabled_Errors(t *testing.T) {
	cmd := newPullCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	_ = cmd.Flags().Set("findings", "false")
	_ = cmd.Flags().Set("corpus", "false")
	cmd.SetContext(context.Background())

	err := runPullWith(cmd, &fakePullCLI{}, &fakeDockerExec{})
	if err == nil {
		t.Fatal("expected error when both --findings=false and --corpus=false")
	}
	if !strings.Contains(err.Error(), "nothing to pull") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFindContainer_MatchesPrefix(t *testing.T) {
	docker := &fakeDockerExec{
		containers: []string{
			"some-other-container",
			"confluence-control--abc123-xyz",
			"another-container",
		},
	}
	name, err := findContainer(context.Background(), docker, "confluence-control--abc123")
	if err != nil {
		t.Fatalf("findContainer: %v", err)
	}
	if name != "confluence-control--abc123-xyz" {
		t.Errorf("unexpected name: %q", name)
	}
}

func TestFindContainer_NotFound_Errors(t *testing.T) {
	docker := &fakeDockerExec{containers: []string{"unrelated"}}
	_, err := findContainer(context.Background(), docker, "confluence-control--missing")
	if err == nil {
		t.Fatal("expected error when container not found")
	}
}

func TestCountFiles(t *testing.T) {
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "a.json"), []byte("a"), 0o644)
	_ = os.WriteFile(filepath.Join(tmp, "b.json"), []byte("b"), 0o644)
	sub := filepath.Join(tmp, "sub")
	_ = os.MkdirAll(sub, 0o755)
	_ = os.WriteFile(filepath.Join(sub, "c.json"), []byte("c"), 0o644)

	n, err := countFiles(tmp)
	if err != nil {
		t.Fatalf("countFiles: %v", err)
	}
	if n != 3 {
		t.Errorf("expected 3, got %d", n)
	}
}
