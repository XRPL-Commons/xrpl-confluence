package main

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/discovery"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/kurtosis"
	"github.com/spf13/cobra"
)

// DockerExec abstracts shelling out to docker, allowing tests to inject a fake.
type DockerExec interface {
	// Ps returns the list of running container names.
	Ps(ctx context.Context) ([]string, error)
	// CopyFromContainer runs docker cp <container>:<src> <dest>.
	CopyFromContainer(ctx context.Context, container, src, dest string) error
}

// realDockerExec delegates to the docker binary on PATH.
type realDockerExec struct{}

func (r *realDockerExec) Ps(ctx context.Context) ([]string, error) {
	out, err := exec.CommandContext(ctx, "docker", "ps", "--format", "{{.Names}}").Output()
	if err != nil {
		return nil, fmt.Errorf("docker ps: %w", err)
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

func (r *realDockerExec) CopyFromContainer(ctx context.Context, container, src, dest string) error {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", "cp", container+":"+src, dest)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker cp %s:%s → %s: %w\n%s", container, src, dest, err, stderr.String())
	}
	return nil
}

func newPullCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Mirror findings + corpus from the enclave to the host",
		RunE:  runPull,
	}
	cmd.Flags().Bool("findings", true, "Mirror /var/confluence/findings → <dest>/findings (default true)")
	cmd.Flags().Bool("corpus", false, "Mirror /output/corpus from the fuzz service → <dest>/corpus (default false)")
	cmd.Flags().String("dest", ".confluence", "Destination root on the host")
	cmd.Flags().String("fuzz-service", "", "Name of the fuzz service for --corpus (auto-detect if empty)")
	return cmd
}

func runPull(cmd *cobra.Command, _ []string) error {
	return runPullWith(cmd, kurtosis.NewExec(), &realDockerExec{})
}

func runPullWith(cmd *cobra.Command, cli kurtosis.CLI, docker DockerExec) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	doFindings, _ := cmd.Flags().GetBool("findings")
	doCorpus, _ := cmd.Flags().GetBool("corpus")
	dest, _ := cmd.Flags().GetString("dest")
	fuzzService, _ := cmd.Flags().GetString("fuzz-service")

	if !doFindings && !doCorpus {
		return fmt.Errorf("nothing to pull: enable --findings or --corpus")
	}

	// Resolve enclave.
	enclave, err := resolveEnclave(cmd, ctx, cli)
	if err != nil {
		return err
	}

	type result struct {
		Kind  string `json:"kind"`
		Count int    `json:"count"`
		Dest  string `json:"dest"`
	}
	var copied []result

	if doFindings {
		svc, err := kurtosis.InspectService(ctx, cli, enclave, "confluence-control")
		if err != nil {
			return fmt.Errorf("inspect confluence-control: %w", err)
		}
		container, err := findContainer(ctx, docker, "confluence-control--"+svc.UUID)
		if err != nil {
			return fmt.Errorf("find confluence-control container: %w", err)
		}

		findingsDest := filepath.Join(dest, "findings")
		if err := os.MkdirAll(findingsDest, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", findingsDest, err)
		}
		if err := docker.CopyFromContainer(ctx, container, "/var/confluence/findings/.", findingsDest); err != nil {
			return err
		}
		count, _ := countFiles(findingsDest)
		copied = append(copied, result{Kind: "findings", Count: count, Dest: findingsDest})

		// Also mirror reproducers from the same container.
		reproducersDest := filepath.Join(dest, "reproducers")
		if err := os.MkdirAll(reproducersDest, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", reproducersDest, err)
		}
		if err := docker.CopyFromContainer(ctx, container, "/var/confluence/reproducers/.", reproducersDest); err != nil {
			return err
		}
		rCount, _ := countFiles(reproducersDest)
		copied = append(copied, result{Kind: "reproducers", Count: rCount, Dest: reproducersDest})
	}

	if doCorpus {
		if fuzzService == "" {
			fuzzService, err = detectFuzzService(ctx, cli, enclave)
			if err != nil {
				return fmt.Errorf("detect fuzz service: %w", err)
			}
		}
		svc, err := kurtosis.InspectService(ctx, cli, enclave, fuzzService)
		if err != nil {
			return fmt.Errorf("inspect %s: %w", fuzzService, err)
		}
		container, err := findContainer(ctx, docker, fuzzService+"--"+svc.UUID)
		if err != nil {
			return fmt.Errorf("find %s container: %w", fuzzService, err)
		}
		corpusDest := filepath.Join(dest, "corpus")
		if err := os.MkdirAll(corpusDest, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", corpusDest, err)
		}
		if err := docker.CopyFromContainer(ctx, container, "/output/corpus/.", corpusDest); err != nil {
			return err
		}
		count, _ := countFiles(corpusDest)
		copied = append(copied, result{Kind: "corpus", Count: count, Dest: corpusDest})
	}

	if jsonMode(cmd) {
		return emitJSON(cmd, map[string]any{
			"enclave_id": enclave,
			"copied":     copied,
		})
	}

	for _, r := range copied {
		fmt.Fprintf(cmd.OutOrStdout(), "Pulled %s: %d files → %s\n", r.Kind, r.Count, r.Dest)
	}
	return nil
}

// resolveEnclave returns the enclave name from --enclave flag or discovery file.
func resolveEnclave(cmd *cobra.Command, ctx context.Context, cli kurtosis.CLI) (string, error) {
	if enclave, _ := cmd.Root().PersistentFlags().GetString("enclave"); enclave != "" {
		return enclave, nil
	}
	cur, err := discovery.Read()
	if err != nil {
		return "", fmt.Errorf("no --enclave flag and no discovery file: %w", err)
	}
	if cur.EnclaveID == "" {
		return "", fmt.Errorf("discovery file has no enclave_id; re-run with --enclave")
	}
	return cur.EnclaveID, nil
}

// findContainer scans docker ps output for a container whose name starts with prefix.
func findContainer(ctx context.Context, docker DockerExec, prefix string) (string, error) {
	names, err := docker.Ps(ctx)
	if err != nil {
		return "", err
	}
	for _, name := range names {
		if strings.HasPrefix(name, prefix) {
			return name, nil
		}
	}
	return "", fmt.Errorf("no running container with prefix %q", prefix)
}

// detectFuzzService returns the first service name starting with "fuzz-" in the enclave.
func detectFuzzService(ctx context.Context, cli kurtosis.CLI, enclave string) (string, error) {
	info, err := kurtosis.InspectEnclave(ctx, cli, enclave)
	if err != nil {
		return "", err
	}
	for _, svc := range info.Services {
		if strings.HasPrefix(svc.Name, "fuzz-") {
			return svc.Name, nil
		}
	}
	return "", fmt.Errorf("no fuzz-* service found in enclave %q", enclave)
}

// countFiles returns the number of regular files under dir.
func countFiles(dir string) (int, error) {
	count := 0
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			count++
		}
		return nil
	})
	return count, err
}
