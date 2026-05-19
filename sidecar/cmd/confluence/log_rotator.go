package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/kurtosis"
)

// startLogRotator tails the kurtosis logs for every service in the enclave
// into per-service files under outDir, rotating each file when it exceeds
// rotateBytes. Built for overnight runs where the in-container ring buffer
// alone won't survive a crash post-mortem (issue 5).
//
// The rotator is best-effort: a failure to enumerate services or to tail one
// service is logged to errOut and never aborts the run. Returns a stop func
// that callers must invoke (typically via defer) to flush + close every tail.
func startLogRotator(ctx context.Context, cli kurtosis.CLI, errOut io.Writer, enclave, outDir string) func() {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(errOut, "rotate-logs: mkdir %s: %v\n", outDir, err)
		return func() {}
	}

	tailCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup

	go func() {
		// Re-enumerate services every 30s so containers that come up later
		// (rebuilds after a crash, lazy sidecars) still get captured.
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		started := make(map[string]bool)
		enumerate := func() {
			info, err := kurtosis.InspectEnclave(tailCtx, cli, enclave)
			if err != nil {
				return
			}
			for _, svc := range info.Services {
				name := svc.Name
				if name == "" || started[name] {
					continue
				}
				started[name] = true
				wg.Add(1)
				go func(svc string) {
					defer wg.Done()
					tailServiceLogs(tailCtx, cli, errOut, enclave, svc, outDir)
				}(name)
			}
		}
		enumerate()
		for {
			select {
			case <-tailCtx.Done():
				return
			case <-ticker.C:
				enumerate()
			}
		}
	}()

	return func() {
		cancel()
		// Bound the shutdown wait so a stuck tail can't hang teardown.
		done := make(chan struct{})
		go func() { wg.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	}
}

// rotateAt is the per-service log rotation threshold. 50 MiB strikes a
// balance: small enough to keep an editor responsive when post-morteming,
// large enough that an active 8h soak doesn't generate hundreds of files.
const rotateAt int64 = 50 * 1024 * 1024

// tailServiceLogs runs `kurtosis service logs -f <enclave> <svc>` and writes
// the output to outDir/<svc>.log, rotating to <svc>.log.<n> at rotateAt.
// Returns when ctx is cancelled or the underlying command exits.
func tailServiceLogs(ctx context.Context, cli kurtosis.CLI, errOut io.Writer, enclave, svc, outDir string) {
	logPath := filepath.Join(outDir, svc+".log")
	f, err := openRotating(logPath)
	if err != nil {
		fmt.Fprintf(errOut, "rotate-logs: open %s: %v\n", logPath, err)
		return
	}
	defer f.Close()

	pr, pw := io.Pipe()
	go func() {
		// kurtosis logs -f streams forever; the pipe close on ctx cancel
		// unblocks the cli.Run when our wrapper context kills the process.
		err := cli.Run(ctx, []string{"service", "logs", "-f", enclave, svc}, nil, pw, pw)
		_ = pw.CloseWithError(err)
	}()

	sc := bufio.NewScanner(pr)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if _, err := f.WriteLine(line); err != nil {
			fmt.Fprintf(errOut, "rotate-logs: write %s: %v\n", logPath, err)
			return
		}
	}
}

// rotatingFile is a tiny line-oriented writer that rotates the active file
// when it crosses rotateAt bytes. Rotated files are renamed with a numeric
// suffix (.log.1, .log.2, ...) so chronological order is preserved by name.
type rotatingFile struct {
	mu      sync.Mutex
	path    string
	file    *os.File
	written int64
	rotated int
}

func openRotating(path string) (*rotatingFile, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return &rotatingFile{path: path, file: f, written: info.Size()}, nil
}

func (r *rotatingFile) WriteLine(line []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.written >= rotateAt {
		if err := r.rotateLocked(); err != nil {
			return 0, err
		}
	}
	n, err := r.file.Write(append(line, '\n'))
	r.written += int64(n)
	return n, err
}

func (r *rotatingFile) rotateLocked() error {
	if err := r.file.Close(); err != nil {
		return err
	}
	r.rotated++
	rotatedPath := fmt.Sprintf("%s.%d", r.path, r.rotated)
	if err := os.Rename(r.path, rotatedPath); err != nil {
		// Re-open the original anyway so we don't drop the next batch on
		// the floor if rename failed (e.g. cross-device).
		f, _ := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		r.file = f
		r.written = 0
		return err
	}
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	r.file = f
	r.written = 0
	return nil
}

func (r *rotatingFile) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return nil
	}
	return r.file.Close()
}
