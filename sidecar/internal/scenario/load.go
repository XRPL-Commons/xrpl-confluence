// Package scenario loads, validates, and compiles confluence Scenario YAML.
package scenario

import (
	"fmt"
	"os"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
	"gopkg.in/yaml.v3"
)

// Load reads a Scenario YAML file from disk.
func Load(path string) (*api.Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("scenario: read %s: %w", path, err)
	}
	return Parse(data)
}

// Parse decodes Scenario YAML from bytes.
func Parse(data []byte) (*api.Scenario, error) {
	var s api.Scenario
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("scenario: parse yaml: %w", err)
	}
	return &s, nil
}
