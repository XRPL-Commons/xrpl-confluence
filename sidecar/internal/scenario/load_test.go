package scenario

import (
	"strings"
	"testing"
)

func TestLoadFile(t *testing.T) {
	s, err := Load("testdata/soak.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Metadata.Name != "soak-mixed-3x2" {
		t.Fatalf("name: %q", s.Metadata.Name)
	}
	if s.Workload.Kind != "soak" {
		t.Fatalf("workload.kind: %q", s.Workload.Kind)
	}
}

func TestParseRejectsMalformedYAML(t *testing.T) {
	_, err := Parse([]byte("not: [valid"))
	if err == nil {
		t.Fatalf("expected error on malformed YAML")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("testdata/does-not-exist.yaml")
	if err == nil {
		t.Fatalf("expected error on missing file")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Fatalf("error should mention path: %v", err)
	}
}
