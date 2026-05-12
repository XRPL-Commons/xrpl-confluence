package kurtosis

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const serviceInspectOutput = `Name: rippled-validator
UUID: abc123def456
Status: RUNNING
IP Address: 172.16.0.5
Ports:
  rpc: 5005/tcp -> 127.0.0.1:12345
`

func TestInspectService_ParsesFields(t *testing.T) {
	f := &fakeCLI{
		next: func(args []string) (string, string, error) {
			return serviceInspectOutput, "", nil
		},
	}
	info, err := InspectService(context.Background(), f, "my-enc", "rippled-validator")
	if err != nil {
		t.Fatalf("InspectService: %v", err)
	}
	if info.UUID != "abc123def456" {
		t.Errorf("UUID: got %q, want %q", info.UUID, "abc123def456")
	}
	if info.IPAddress != "172.16.0.5" {
		t.Errorf("IPAddress: got %q, want %q", info.IPAddress, "172.16.0.5")
	}
	if info.Name != "rippled-validator" {
		t.Errorf("Name: got %q", info.Name)
	}
}

func TestInspectService_MissingFields(t *testing.T) {
	f := &fakeCLI{
		next: func(args []string) (string, string, error) {
			return "Status: RUNNING\n", "", nil
		},
	}
	_, err := InspectService(context.Background(), f, "enc", "svc")
	if err == nil {
		t.Fatal("expected error for missing UUID and IP Address")
	}
}

const enclaveInspectOutput = `Enclave ID:                                           my-enclave

Services:
Name                  UUID          Status
rippled-validator     abc123        RUNNING
rippled-peer          def456        RUNNING
`

func TestInspectEnclave_ParsesServices(t *testing.T) {
	f := &fakeCLI{
		next: func(args []string) (string, string, error) {
			return enclaveInspectOutput, "", nil
		},
	}
	info, err := InspectEnclave(context.Background(), f, "my-enclave")
	if err != nil {
		t.Fatalf("InspectEnclave: %v", err)
	}
	if len(info.Services) != 2 {
		t.Fatalf("expected 2 services, got %d: %v", len(info.Services), info.Services)
	}
	if info.Services[0].Name != "rippled-validator" {
		t.Errorf("Services[0].Name: got %q", info.Services[0].Name)
	}
	if info.Services[1].Name != "rippled-peer" {
		t.Errorf("Services[1].Name: got %q", info.Services[1].Name)
	}
}

func TestRemoveEnclave_NotFound(t *testing.T) {
	f := &fakeCLI{
		next: func(args []string) (string, string, error) {
			return "", "enclave not found", errors.New("exit status 1")
		},
	}
	if err := RemoveEnclave(context.Background(), f, "missing-enc"); err != nil {
		t.Fatalf("RemoveEnclave should swallow not-found: %v", err)
	}
}

func TestRemoveEnclave_OtherError(t *testing.T) {
	f := &fakeCLI{
		next: func(args []string) (string, string, error) {
			return "", "permission denied", errors.New("exit status 2")
		},
	}
	err := RemoveEnclave(context.Background(), f, "enc")
	if err == nil {
		t.Fatal("expected error for non-not-found failure")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error should mention stderr: %v", err)
	}
}

const enclaveListOutput = `UUID           Name          Status    Creation Time
aaabbbccc111   my-enclave    RUNNING   Tue, 12 May 2026 18:16:03 CEST
dddeeefff222   other-enc     STOPPED   Mon, 11 May 2026 09:00:00 CEST
`

func TestListEnclaves_ParsesRows(t *testing.T) {
	f := &fakeCLI{
		next: func(args []string) (string, string, error) {
			return enclaveListOutput, "", nil
		},
	}
	names, err := ListEnclaves(context.Background(), f)
	if err != nil {
		t.Fatalf("ListEnclaves: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 enclaves, got %d: %v", len(names), names)
	}
	if names[0] != "my-enclave" || names[1] != "other-enc" {
		t.Errorf("unexpected names: %v", names)
	}
}
