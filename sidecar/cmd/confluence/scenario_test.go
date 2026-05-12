package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runCmd(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	outBuf, errBuf := &bytes.Buffer{}, &bytes.Buffer{}
	root := newRootCmd()
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	root.SetArgs(args)
	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

func writeTempYAML(t *testing.T, body string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "scenario-*.yaml")
	if err != nil {
		t.Fatalf("temp: %v", err)
	}
	if _, err := f.WriteString(body); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = f.Close()
	return f.Name()
}

func TestScenarioValidateValid(t *testing.T) {
	path := filepath.Join("..", "..", "internal", "scenario", "testdata", "soak.yaml")
	stdout, _, err := runCmd(t, "scenario", "validate", "--json", path)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got struct {
		OK     bool             `json:"ok"`
		Errors []map[string]any `json:"errors"`
	}
	if jerr := json.Unmarshal([]byte(stdout), &got); jerr != nil {
		t.Fatalf("not JSON: %v (out=%q)", jerr, stdout)
	}
	if !got.OK || len(got.Errors) != 0 {
		t.Fatalf("expected ok with no errors, got %+v", got)
	}
}

func TestScenarioValidateInvalid(t *testing.T) {
	path := writeTempYAML(t, "kind: NotScenario\n")
	stdout, _, err := runCmd(t, "scenario", "validate", "--json", path)
	if err == nil {
		t.Fatalf("expected non-nil error on invalid scenario")
	}
	if !strings.Contains(stdout, `"ok":false`) {
		t.Fatalf("expected ok:false in output, got %q", stdout)
	}
	var got struct {
		OK     bool             `json:"ok"`
		Errors []map[string]any `json:"errors"`
	}
	if jerr := json.Unmarshal([]byte(stdout), &got); jerr != nil {
		t.Fatalf("not JSON: %v", jerr)
	}
	if got.OK || len(got.Errors) == 0 {
		t.Fatalf("expected errors, got %+v", got)
	}
}

func TestScenarioValidateHumanOutput(t *testing.T) {
	path := writeTempYAML(t, "kind: NotScenario\n")
	stdout, _, err := runCmd(t, "scenario", "validate", path)
	if err == nil {
		t.Fatalf("expected non-nil error")
	}
	if !strings.Contains(stdout, "kind") {
		t.Fatalf("expected human output to reference the bad field, got %q", stdout)
	}
}
