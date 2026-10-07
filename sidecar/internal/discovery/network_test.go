package discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNetworkRecordsAreSeparateAndConfined(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, name := range []string{"team-one", "team-two", "../../escape"} {
		data, _ := json.Marshal(map[string]string{"name": name})
		if err := WriteNetwork(name, data); err != nil {
			t.Fatal(err)
		}
		got, err := ReadNetwork(name)
		if err != nil || string(got) != string(data) {
			t.Fatalf("record %q: %s %v", name, got, err)
		}
	}
	if err := RemoveNetwork("team-one"); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadNetwork("team-two"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(".confluence", "networks"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("expected two independent confined records: %v %v", entries, err)
	}
}
