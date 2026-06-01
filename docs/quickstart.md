---
description: Boot your first mixed rippled + go-xrpl network and run an interop suite.
---

# Quickstart

<DownloadLLMsFullDoc />

This guide gets a mixed network running and shows the two ways to drive it: the **`confluence` CLI**
(preferred, scenario-driven) and the **legacy Makefile** (`make soak` / `make chaos`).

## Prerequisites

- **[Kurtosis CLI](https://docs.kurtosis.com/install/)** installed, with the engine running
  (`kurtosis engine status`).
- **Docker** running locally — nodes and the sidecar run as containers.
- **Go 1.25+** to build the `confluence` CLI.
- Node images available locally or pullable: `rippleci/rippled:2.6.2` (default) and `goxrpl:latest`.

> If a prerequisite is missing, install it first — Confluence will not auto-install Docker or Kurtosis.

All commands run from the repository root.

## Option A — the `confluence` CLI (recommended)

### 1. Build the CLI

The binary is not pre-installed (`bin/` is gitignored):

```bash
( cd sidecar && go build -o ../bin/confluence ./cmd/confluence )
export PATH="$PWD/bin:$PATH"   # or invoke as ./bin/confluence
confluence version
```

### 2. Validate and boot a scenario

```bash
confluence scenario validate scenarios/soak-mixed-3x2.yaml
confluence up -f scenarios/soak-mixed-3x2.yaml
confluence status -w          # live network status, refreshes every 2s
```

`scenarios/soak-mixed-3x2.yaml` brings up **3 rippled + 2 go-xrpl** validators and runs a soak
workload with a 10-minute budget that stops on the first divergence. See
[CLI & Scenarios](/cli) for the Scenario format.

### 3. Watch findings

```bash
confluence findings --limit 50
confluence finding show <id>
confluence events | jq 'select(.type=="finding")'
```

### 4. Pull results and tear down

```bash
confluence pull --corpus      # → .confluence/findings + .confluence/corpus
confluence down               # tear down the current enclave
```

### One-shot

To boot, run until the budget elapses (or a `stop_on` predicate fires), and tear down — all in one
command:

```bash
confluence run -f scenarios/soak-mixed-3x2.yaml
```

## Option B — the legacy Makefile

The Makefile predates the CLI and is kept for backward compatibility.

### Soak

```bash
make soak                                  # 2 go-xrpl + 3 rippled, tx_rate=5 (defaults)
make soak GOXRPL_COUNT=3 RIPPLED_COUNT=5 TX_RATE=10 OBSERVABILITY=1
make soak-status                           # inspect the enclave
make soak-tail                             # stream fuzz-soak logs
make soak-pull                             # copy /output/corpus → ./.soak-corpus
make soak-down                             # tear down
```

### Chaos

Chaos needs the host-level docker-socket-proxy (one-time per host) and a schedule file:

```bash
make docker-proxy                          # expose Docker on tcp://host.docker.internal:2375
cp .chaos-schedule.example.json .chaos-schedule.json
make chaos                                 # reads .chaos-schedule.json
make chaos-tail / chaos-pull / chaos-down
```

Tunables (Make vars): `ENCLAVE`, `GOXRPL_COUNT`, `RIPPLED_COUNT`, `TX_RATE`, `ACCOUNTS`,
`ROTATE_EVERY`, `MUTATION_RATE`, `CORPUS`, `OBSERVABILITY`, `ALERT_WEBHOOK_URL`, `CHAOS_SCHEDULE`.

## Running a network directly with `main.star`

Both surfaces ultimately call `main.star`. You can invoke it directly with `kurtosis run`:

```bash
kurtosis run --enclave my-net . '{
  "test_suite": "consensus",
  "rippled_count": 4,
  "goxrpl_count": 1
}'
```

`run(plan, args)` accepts:

| Arg | Default | Meaning |
| --- | --- | --- |
| `rippled_count` | `4` | Number of rippled nodes |
| `goxrpl_count` | `1` | Number of go-xrpl nodes |
| `rippled_image` | `rippleci/rippled:2.6.2` | rippled image |
| `goxrpl_image` | `goxrpl:latest` | go-xrpl image |
| `test_suite` | `all` | `all` · `propagation` · `sync` · `consensus` · `soak` · `delayed_sync` · `fuzz` · `replay` · `shrink` · `chaos` |
| `soak_args` | — | `tx_rate`, `rotate_every`, `mutation_rate`, `accounts` (soak only) |
| `chaos_args` | — | `schedule` (JSON string, required) + soak args (chaos only) |
| `shrink_args` | — | `shrink_artifact`, `shrink_max_step` (shrink only) |

A pre-flight check rejects impossible topologies before any container starts: at least **2 total
nodes** are required for oracle comparison, and `soak` / `chaos` / `shrink` require **≥ 2 rippled and
≥ 1 go-xrpl**, while `fuzz` / `replay` require **≥ 2 rippled**.

## Next steps

- [Test Suites](/test-suites) — pick the right suite for what you want to prove.
- [Sidecar & Oracle](/sidecar-oracle) — how divergence is detected.
- [Dashboard](/dashboard) — watch the network live.
