package discovery

import (
	"errors"
	"io/fs"
	"testing"
	"time"
)

func TestRead_Absent(t *testing.T) {
	t.Chdir(t.TempDir())
	c, err := Read()
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected fs.ErrNotExist, got %v", err)
	}
	if c != nil {
		t.Fatal("expected nil Current")
	}
}

func TestWriteRead_RoundTrip(t *testing.T) {
	t.Chdir(t.TempDir())
	want := &Current{
		EnclaveID:  "enc-123",
		ControlURL: "http://192.168.1.42:8090",
		Scenario:   "soak",
		StartedAt:  time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	if err := Write(want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.EnclaveID != want.EnclaveID {
		t.Errorf("EnclaveID: got %q, want %q", got.EnclaveID, want.EnclaveID)
	}
	if got.ControlURL != want.ControlURL {
		t.Errorf("ControlURL: got %q, want %q", got.ControlURL, want.ControlURL)
	}
	if got.Scenario != want.Scenario {
		t.Errorf("Scenario: got %q, want %q", got.Scenario, want.Scenario)
	}
	if !got.StartedAt.Equal(want.StartedAt) {
		t.Errorf("StartedAt: got %v, want %v", got.StartedAt, want.StartedAt)
	}
}

func TestWrite_CreatesDir(t *testing.T) {
	t.Chdir(t.TempDir())
	c := &Current{EnclaveID: "enc-abc", StartedAt: time.Now()}
	if err := Write(c); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := Read(); err != nil {
		t.Fatalf("Read after write: %v", err)
	}
}

func TestRemove_Absent(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := Remove(); err != nil {
		t.Fatalf("Remove on absent file: %v", err)
	}
}

func TestRemove_DeletesFile(t *testing.T) {
	t.Chdir(t.TempDir())
	c := &Current{EnclaveID: "enc-del", StartedAt: time.Now()}
	if err := Write(c); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	_, err := Read()
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected fs.ErrNotExist after Remove, got %v", err)
	}
}
