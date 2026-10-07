package scenario

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
)

func TestCompileSoakGolden(t *testing.T) {
	s, err := Load("testdata/soak.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	got, err := Compile(s)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	assertJSONEqualToFile(t, got, "testdata/soak-compiled.json")
}

func TestCompileChaosGolden(t *testing.T) {
	s, err := Load("testdata/chaos.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	got, err := Compile(s)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	assertJSONEqualToFile(t, got, "testdata/chaos-compiled.json")
}

func TestCompileLocalNetworkGolden(t *testing.T) {
	s, err := Load("testdata/none-rippled.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	got, err := Compile(s)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	assertJSONEqualToFile(t, got, "testdata/none-rippled-compiled.json")
	var args map[string]any
	if err := json.Unmarshal(got, &args); err != nil {
		t.Fatalf("unmarshal compiled args: %v", err)
	}
	for _, key := range []string{"soak_args", "chaos_args"} {
		if _, ok := args[key]; ok {
			t.Fatalf("none workload must not emit %s: %s", key, got)
		}
	}
}

func TestCompileRejectsInvalidScenario(t *testing.T) {
	s, err := Load("testdata/soak.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	s.Metadata.Name = "" // make it invalid
	if _, err := Compile(s); err == nil {
		t.Fatalf("expected compile to reject invalid scenarios")
	}
}

func TestCompileRejectsReplay(t *testing.T) {
	// Replay scenarios are not compiled directly; they flow through
	// `confluence replay`, which composes its own kurtosis input. The
	// scenario below is valid (passes Validate) so we exercise the explicit
	// guard inside Compile, not the validator's replay rule.
	s, err := Load("testdata/soak.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	s.Workload.Kind = "replay"
	s.Workload.Reproducer = &api.WorkloadReproducer{ID: "rpr_0000000000000000000000000"}
	if _, err := Compile(s); err == nil || !strings.Contains(err.Error(), "replay") {
		t.Fatalf("expected replay rejection from Compile guard, got err=%v", err)
	}
}

func TestCompileOmitsEmptyImagesForStarlarkDefaults(t *testing.T) {
	s := validScenario()
	s.Topology.Rippled.Image = ""
	s.Topology.Goxrpl.Image = ""
	got, err := Compile(s)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	var args map[string]any
	if err := json.Unmarshal(got, &args); err != nil {
		t.Fatalf("unmarshal compiled args: %v", err)
	}
	for _, key := range []string{"rippled_image", "goxrpl_image"} {
		if _, ok := args[key]; ok {
			t.Fatalf("empty image must be omitted from %s: %s", key, got)
		}
	}
}

func assertJSONEqualToFile(t *testing.T, got []byte, path string) {
	t.Helper()
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	if !jsonEqual(t, got, want) {
		t.Fatalf("compile mismatch\n got: %s\nwant: %s", got, bytes.TrimSpace(want))
	}
}

func jsonEqual(t *testing.T, a, b []byte) bool {
	t.Helper()
	var ax, bx any
	if err := json.Unmarshal(a, &ax); err != nil {
		t.Fatalf("got is not JSON: %v (%s)", err, a)
	}
	if err := json.Unmarshal(b, &bx); err != nil {
		t.Fatalf("want is not JSON: %v", err)
	}
	ja, _ := json.Marshal(ax)
	jb, _ := json.Marshal(bx)
	return bytes.Equal(ja, jb)
}
