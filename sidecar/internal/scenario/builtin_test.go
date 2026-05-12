package scenario

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuiltinScenariosLoadAndCompile guards every YAML under ../../../scenarios/
// against drift: each file must load, validate, and (unless it's a replay
// template) compile cleanly. New built-ins get this test for free.
func TestBuiltinScenariosLoadAndCompile(t *testing.T) {
	root := filepath.Join("..", "..", "..", "scenarios")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Skipf("no scenarios dir at %s: %v", root, err)
		return
	}
	var found int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		found++
		t.Run(e.Name(), func(t *testing.T) {
			path := filepath.Join(root, e.Name())
			s, err := Load(path)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if errs := Validate(s); len(errs) != 0 {
				t.Fatalf("validate: %+v", errs)
			}
			if s.Workload.Kind == "replay" {
				return // replay templates don't compile
			}
			if _, err := Compile(s); err != nil {
				t.Fatalf("compile: %v", err)
			}
		})
	}
	if found == 0 {
		t.Fatalf("expected at least one built-in scenario under %s", root)
	}
}
