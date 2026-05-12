package discovery

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"
)

const path = ".confluence/current.json"

// Current holds the identity of the active enclave.
type Current struct {
	EnclaveID  string    `json:"enclave_id"`
	ControlURL string    `json:"control_url"`
	Scenario   string    `json:"scenario,omitempty"`
	StartedAt  time.Time `json:"started_at"`
}

// Path returns the relative path of the discovery file.
func Path() string { return path }

// Read reads the discovery file. Returns nil, fs.ErrNotExist when absent.
func Read() (*Current, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fs.ErrNotExist
		}
		return nil, fmt.Errorf("discovery: read current.json: %w", err)
	}
	var c Current
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("discovery: read current.json: %w", err)
	}
	return &c, nil
}

// Write writes c to the discovery file, creating .confluence if needed.
func Write(c *Current) error {
	if err := os.MkdirAll(".confluence", 0o755); err != nil {
		return fmt.Errorf("discovery: write current.json: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("discovery: write current.json: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("discovery: write current.json: %w", err)
	}
	return nil
}

// Remove deletes the discovery file. Missing file is not an error.
func Remove() error {
	err := os.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("discovery: remove current.json: %w", err)
	}
	return nil
}
