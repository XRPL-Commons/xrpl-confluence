// Command forkdebug is a multi-tool for investigating consensus
// forks and stalls in mixed rippled/goXRPL networks.
//
// Subcommands:
//
//	scan      sweep a seq range and report the FIRST divergent ledger
//	          across all configured nodes (with full per-node hash table)
//	isolate   dump the canonical tx-set at a known fork seq from a chosen
//	          source node, in a form suitable for bisection-replay
//	tap       parse goxrpl structured consensus logs from stdin and emit
//	          one-line summaries (close-time votes, mode changes, accept-ct,
//	          ledger-built, validate emit/skip)
//	stalled   poll every node's validated_seq for a window and report
//	          whether the chain is making forward progress
//
// Examples:
//
//	# Find the first fork in seq [1..200] across 5 nodes.
//	forkdebug scan \
//	  --from 1 --to 200 \
//	  --node goxrpl-0=http://127.0.0.1:64627 \
//	  --node goxrpl-1=http://127.0.0.1:64631 \
//	  --node rippled-0=http://127.0.0.1:64537 \
//	  --node rippled-1=http://127.0.0.1:64534 \
//	  --node rippled-2=http://127.0.0.1:64540
//
//	# Dump the 41-tx set at seq=18 from rippled-0.
//	forkdebug isolate --seq 18 \
//	  --node rippled-0=http://127.0.0.1:64537 > seq18.json
//
//	# Tail goxrpl-0 consensus events.
//	docker logs -f goxrpl-0--<uuid> 2>&1 | forkdebug tap
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/forkdebug"
)

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "scan":
		runScan(args)
	case "isolate":
		runIsolate(args)
	case "tap":
		runTap(args)
	case "stalled":
		runStalled(args)
	case "-h", "--help", "help":
		usage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n\n", cmd)
		usage(os.Stderr)
		os.Exit(2)
	}
}

func usage(w *os.File) {
	fmt.Fprint(w, `forkdebug — fork investigation toolkit

USAGE
    forkdebug scan     --from N --to M --node name=URL [--node ...]
    forkdebug isolate  --seq N --node name=URL [--json|--summary]
    forkdebug tap      [< logfile | docker logs ... | forkdebug tap]
    forkdebug stalled  --window 30s --node name=URL [--node ...]

Run "forkdebug <cmd> -h" for subcommand flags.
`)
}

// nodeListFlag accumulates --node name=URL repeated flags.
type nodeListFlag []forkdebug.Node

func (n *nodeListFlag) String() string {
	parts := make([]string, 0, len(*n))
	for _, x := range *n {
		parts = append(parts, fmt.Sprintf("%s=%s", x.Name, x.URL))
	}
	return strings.Join(parts, ",")
}
func (n *nodeListFlag) Set(v string) error {
	i := strings.IndexByte(v, '=')
	if i <= 0 || i == len(v)-1 {
		return fmt.Errorf("--node must be name=URL, got %q", v)
	}
	*n = append(*n, forkdebug.Node{Name: v[:i], URL: v[i+1:]})
	return nil
}

func runScan(args []string) {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	from := fs.Int("from", 1, "starting ledger sequence (inclusive)")
	to := fs.Int("to", 200, "ending ledger sequence (inclusive)")
	asJSON := fs.Bool("json", false, "emit JSON instead of human-readable summary")
	var nodes nodeListFlag
	fs.Var(&nodes, "node", "node spec, name=URL (repeat flag for each node, min 2)")
	_ = fs.Parse(args)

	if len(nodes) < 2 {
		fmt.Fprintln(os.Stderr, "scan: need at least 2 --node flags")
		fs.Usage()
		os.Exit(2)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	scanner, err := forkdebug.NewScanner(nodes)
	if err != nil {
		fmt.Fprintln(os.Stderr, "scan:", err)
		os.Exit(1)
	}
	result := scanner.FindFirstFork(ctx, *from, *to)

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(result)
		if result.FirstFork != nil {
			os.Exit(1) // non-zero exit when a fork was found, for CI
		}
		return
	}

	fmt.Print(forkdebug.FormatScanResult(result))
	if result.FirstFork != nil {
		os.Exit(1)
	}
}

func runIsolate(args []string) {
	fs := flag.NewFlagSet("isolate", flag.ExitOnError)
	seq := fs.Int("seq", 0, "ledger sequence to isolate (the divergent seq)")
	asSummary := fs.Bool("summary", false, "print one-line summary instead of full JSON")
	var nodes nodeListFlag
	fs.Var(&nodes, "node", "single source node, name=URL (the canonical/healthy side)")
	_ = fs.Parse(args)

	if *seq <= 0 {
		fmt.Fprintln(os.Stderr, "isolate: --seq required (>0)")
		os.Exit(2)
	}
	if len(nodes) != 1 {
		fmt.Fprintln(os.Stderr, "isolate: need exactly one --node")
		os.Exit(2)
	}

	res, err := forkdebug.IsolateAtSeq(nodes[0], *seq)
	if err != nil {
		fmt.Fprintln(os.Stderr, "isolate:", err)
		os.Exit(1)
	}

	if *asSummary {
		fmt.Printf("seq=%d source=%s ledger=%s acct=%s tx_root=%s tx_count=%d close_time=%d close_flags=%d\n",
			res.Sequence, res.SourceNode,
			res.LedgerHash, res.AccountHash, res.TransactionRoot,
			res.TxCount, res.CloseTime, res.CloseFlags)
		return
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(res)
}

func runTap(args []string) {
	fs := flag.NewFlagSet("tap", flag.ExitOnError)
	_ = fs.Parse(args)
	if err := forkdebug.Tap(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "tap:", err)
		os.Exit(1)
	}
}

func runStalled(args []string) {
	fs := flag.NewFlagSet("stalled", flag.ExitOnError)
	window := fs.Duration("window", 30*time.Second, "observation window (e.g. 30s, 2m)")
	interval := fs.Duration("interval", 3*time.Second, "poll interval inside the window")
	asJSON := fs.Bool("json", false, "emit JSON instead of human-readable summary")
	var nodes nodeListFlag
	fs.Var(&nodes, "node", "node spec, name=URL (repeat flag for each node, min 1)")
	_ = fs.Parse(args)

	if len(nodes) < 1 {
		fmt.Fprintln(os.Stderr, "stalled: need at least 1 --node flag")
		fs.Usage()
		os.Exit(2)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	det, err := forkdebug.NewStallDetector(nodes)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stalled:", err)
		os.Exit(1)
	}
	res := det.Watch(ctx, *window, *interval)

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
	} else {
		fmt.Print(forkdebug.FormatStallResult(res))
	}
	if res.Stalled {
		os.Exit(1) // non-zero so CI can gate on it
	}
}
