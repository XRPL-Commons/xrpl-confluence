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
	Ports     map[string]int // application port name -> host-side port
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

	info := &ServiceInfo{Name: service}
	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if k, v, ok := cutField(line, "UUID:"); ok && k == "UUID" {
			info.UUID = v
		} else if k, v, ok := cutField(line, "IP Address:"); ok && k == "IP Address" {
			info.IPAddress = v
		}
	}
	if info.UUID == "" && info.IPAddress == "" {
		return nil, fmt.Errorf("kurtosis service inspect %s/%s: UUID and IP Address not found in output", enclave, service)
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
	header := true
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Skip the header row (contains "Name" or dashes).
		if header {
			header = false
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			continue
		}
		names = append(names, fields[0])
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
