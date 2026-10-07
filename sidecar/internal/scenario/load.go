// Package scenario loads, validates, and compiles confluence Scenario YAML.
package scenario

import (
	"bytes"
	"fmt"
	"io"
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
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&s); err != nil {
		return nil, fmt.Errorf("scenario: parse yaml: %w", err)
	}
	// A scenario file is one document. Rejecting trailing documents avoids
	// silently accepting a second scenario that the caller did not intend to
	// run.
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("scenario: parse yaml: multiple documents are not supported")
		}
		return nil, fmt.Errorf("scenario: parse yaml: %w", err)
	}
	return &s, nil
}
