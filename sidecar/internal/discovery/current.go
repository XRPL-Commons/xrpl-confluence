package discovery

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

const path = ".confluence/current.json"

// Current holds the identity of the active enclave.
type Current struct {
	EnclaveID    string         `json:"enclave_id"`
	ControlURL   string         `json:"control_url"`
	Scenario     string         `json:"scenario,omitempty"`
	StartedAt    time.Time      `json:"started_at"`
	DashboardURL string         `json:"dashboard_url,omitempty"`
	Nodes        []NodeEndpoint `json:"nodes,omitempty"`
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

// NodeEndpoint contains host-accessible addresses for one validator.
type NodeEndpoint struct {
	Name string `json:"name"`
	RPC  string `json:"rpc"`
	WS   string `json:"ws"`
	Peer string `json:"peer"`
}

// Write writes c to the discovery file, creating .confluence if needed.
func Write(c *Current) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("discovery: encode: %w", err)
	}
	return writeAtomic(path, data)
}

func networkPath(enclave string) string {
	return filepath.Join(".confluence", "networks", fmt.Sprintf("%x.json", sha256.Sum256([]byte(enclave))))
}

func WriteNetwork(enclave string, args json.RawMessage) error {
	return writeAtomic(networkPath(enclave), args)
}

func ReadNetwork(enclave string) (json.RawMessage, error) {
	data, err := os.ReadFile(networkPath(enclave))
	if err != nil {
		return nil, fmt.Errorf("discovery: network %q: %w", enclave, err)
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("discovery: invalid network configuration for %q", enclave)
	}
	return data, nil
}

func RemoveNetwork(enclave string) error {
	err := os.Remove(networkPath(enclave))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func writeAtomic(name string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return fmt.Errorf("discovery: create directory: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(name), ".discovery-*")
	if err != nil {
		return fmt.Errorf("discovery: create temporary file: %w", err)
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("discovery: write: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("discovery: close: %w", err)
	}
	if err := os.Rename(file.Name(), name); err != nil {
		return fmt.Errorf("discovery: replace: %w", err)
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
