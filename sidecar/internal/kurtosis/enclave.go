package kurtosis

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strings"
)

// ServiceInfo holds runtime details for a single service.
type ServiceInfo struct {
	Name      string
	UUID      string
	IPAddress string
	Ports     map[string]int    // application port name -> host-side port
	PortURLs  map[string]string // application port name -> host-side URL (e.g. http://127.0.0.1:59061)
}

// EnclaveInfo holds the list of services running in an enclave.
type EnclaveInfo struct {
	Name     string
	Services []ServiceInfo
}

// InspectService runs `kurtosis service inspect <enclave> <service>` and
// parses the text output for UUID and IP Address fields.
func InspectService(ctx context.Context, cli CLI, enclave, service string) (*ServiceInfo, error) {
	var stdout, stderr bytes.Buffer
	if err := cli.Run(ctx, []string{"service", "inspect", enclave, service}, nil, &stdout, &stderr); err != nil {
		return nil, fmt.Errorf("kurtosis service inspect: %w\n%s", err, stderr.String())
	}

	info := &ServiceInfo{Name: service, PortURLs: map[string]string{}}
	scanner := bufio.NewScanner(&stdout)
	inPorts := false
	for scanner.Scan() {
		line := scanner.Text()
		if k, v, ok := cutField(line, "UUID:"); ok && k == "UUID" {
			info.UUID = v
			continue
		}
		if k, v, ok := cutField(line, "IP Address:"); ok && k == "IP Address" {
			info.IPAddress = v
			continue
		}
		if strings.TrimSpace(line) == "Ports:" {
			inPorts = true
			continue
		}
		// Stop scanning Ports section at next top-level key.
		if inPorts && len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
			inPorts = false
		}
		if inPorts {
			// Line shape: "  http: 8090/tcp -> http://127.0.0.1:59061"
			trimmed := strings.TrimSpace(line)
			if name, rest, ok := strings.Cut(trimmed, ":"); ok {
				if _, hostURL, ok := strings.Cut(rest, "->"); ok {
					info.PortURLs[strings.TrimSpace(name)] = strings.TrimSpace(hostURL)
				}
			}
		}
	}
	if info.UUID == "" && info.IPAddress == "" && len(info.PortURLs) == 0 {
		return nil, fmt.Errorf("kurtosis service inspect %s/%s: no UUID, IP Address, or Ports found in output", enclave, service)
	}
	return info, nil
}

// InspectEnclave runs `kurtosis enclave inspect <enclave>` and returns the
// list of services with their names populated.
func InspectEnclave(ctx context.Context, cli CLI, enclave string) (*EnclaveInfo, error) {
	var stdout, stderr bytes.Buffer
	if err := cli.Run(ctx, []string{"enclave", "inspect", enclave}, nil, &stdout, &stderr); err != nil {
		return nil, fmt.Errorf("kurtosis enclave inspect: %w\n%s", err, stderr.String())
	}

	info := &EnclaveInfo{Name: enclave}
	scanner := bufio.NewScanner(&stdout)
	inServices := false
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Detect the services section header.
		if strings.Contains(strings.ToLower(line), "services") {
			inServices = true
			continue
		}
		if !inServices {
			continue
		}
		// Skip separator/header rows (contain dashes or column headers like "Name").
		if strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "Name") || strings.HasPrefix(trimmed, "UUID") {
			continue
		}
		// Each service row: first whitespace-separated field is the name.
		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			continue
		}
		info.Services = append(info.Services, ServiceInfo{Name: fields[0]})
	}
	return info, nil
}

// RemoveEnclave runs `kurtosis enclave rm -f <enclave>`. A non-existent
// enclave is treated as success.
func RemoveEnclave(ctx context.Context, cli CLI, enclave string) error {
	var stdout, stderr bytes.Buffer
	err := cli.Run(ctx, []string{"enclave", "rm", "-f", enclave}, nil, &stdout, &stderr)
	if err == nil {
		return nil
	}
	errText := strings.ToLower(stderr.String())
	if strings.Contains(errText, "not found") ||
		strings.Contains(errText, "no such enclave") ||
		strings.Contains(errText, "doesn't exist") {
		return nil
	}
	return fmt.Errorf("kurtosis enclave rm: %w\n%s", err, stderr.String())
}

// ListEnclaves runs `kurtosis enclave ls` and returns enclave names.
func ListEnclaves(ctx context.Context, cli CLI) ([]string, error) {
	var stdout, stderr bytes.Buffer
	if err := cli.Run(ctx, []string{"enclave", "ls"}, nil, &stdout, &stderr); err != nil {
		return nil, fmt.Errorf("kurtosis enclave ls: %w\n%s", err, stderr.String())
	}

	var names []string
	scanner := bufio.NewScanner(&stdout)
	nameIdx := 1 // default: Name is the second column (after UUID)
	headerParsed := false
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !headerParsed {
			headerParsed = true
			// Determine which column is "Name".
			for i, f := range strings.Fields(trimmed) {
				if strings.EqualFold(f, "name") {
					nameIdx = i
					break
				}
			}
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) <= nameIdx {
			continue
		}
		names = append(names, fields[nameIdx])
	}
	return names, nil
}

// cutField splits a line like "Key: value" on sep and returns (key, value, true).
func cutField(line, sep string) (string, string, bool) {
	idx := strings.Index(line, sep)
	if idx < 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:idx+len(sep)-1]) // strip trailing colon
	val := strings.TrimSpace(line[idx+len(sep):])
	return key, val, true
}
