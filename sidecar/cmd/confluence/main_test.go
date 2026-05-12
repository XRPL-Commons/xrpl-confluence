package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestVersionJSON(t *testing.T) {
	out := &bytes.Buffer{}
	root := newRootCmd()
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs([]string{"version", "--json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var got struct {
		Version    string `json:"version"`
		APIVersion string `json:"api_version"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("not JSON: %v (out=%q)", err, out.String())
	}
	if got.APIVersion != "confluence/v1" {
		t.Fatalf("api_version: got %q want %q", got.APIVersion, "confluence/v1")
	}
	if got.Version == "" {
		t.Fatalf("version: empty")
	}
}
