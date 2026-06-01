---
description: The confluence CLI and the declarative Scenario YAML format.
---

# CLI & Scenarios

<DownloadLLMsFullDoc />

There are two ways to drive Confluence: the **`confluence` CLI** (preferred, scenario-driven) and the
**legacy Makefile** (`make soak` / `make chaos`, covered in the [Quickstart](/quickstart)). This page
documents the CLI and the Scenario format it consumes.

## Building the CLI

The Cobra binary lives at `sidecar/cmd/confluence/` and is not pre-installed (`bin/` is gitignored):

```bash
( cd sidecar && go build -o ../bin/confluence ./cmd/confluence )
export PATH="$PWD/bin:$PATH"   # or invoke as ./bin/confluence
confluence version
```

Rebuild after pulling.

## Global flags

These work on every subcommand:

| Flag | Meaning |
| --- | --- |
| `--enclave <name>` | Target a specific enclave (defaults to the current-context enclave) |
| `--control-url http://host:port` | Hit the control service directly, bypassing the Kurtosis lookup |
| `--json` | Emit machine-readable JSON/NDJSON instead of human tables (for `jq`) |

## Subcommands

| Command | Purpose |
| --- | --- |
| `confluence up -f scenarios/foo.yaml` | Boot an enclave from a Scenario YAML |
| `confluence down [ENCLAVE]` | Tear down the current (or named) enclave |
| `confluence ls` | List all confluence enclaves |
| `confluence status [-w]` | Network status of the current enclave; `-w` refreshes every 2 s |
| `confluence run SCENARIO` | Boot + run + wait for budget/`stop_on` + optional tear-down (one-shot) |
| `confluence replay REPRODUCER_ID` | Boot an enclave from a saved reproducer YAML |
| `confluence logs -n NODE [-f] [--since 10m] [--grep regex]` | Stream a node's logs |
| `confluence events` | Stream control-service SSE events as NDJSON |
| `confluence findings [--kind K] [--since ID] [--limit N]` | List findings from the running enclave |
| `confluence finding show ID` | Show one finding in detail |
| `confluence pull [--dest .confluence] [--corpus] [--no-findings]` | Mirror findings (and optionally corpus) to the host |
| `confluence scenario validate PATH` | Validate a Scenario YAML before booting |

### `up` flags

`-f, --scenario PATH` (required), `--enclave NAME`, `--package DIR` (default `.`),
`--tear-down-first` (default true), `--wait-control 60s`, and:

- `--boot-hang-threshold 90s` — kill the Kurtosis CLI if it stays silent past this (watchdog for the
  occasional 0% CPU boot hang; `0` disables).
- `--rebuild-goxrpl PATH` — `docker build` PATH and tag it as `topology.goxrpl.image` before booting.
- `--rebuild-rippled PATH` — same idea for `topology.rippled.image`.
- `--with-dashboard` — force `observability.enabled=true` regardless of the YAML.

### `run` flags

All `up` flags **plus**:

- `-w, --wait` (default true), `--timeout DUR` (defaults to 2× the scenario budget),
  `--down` (default true — tear down on finish).
- `--budget DUR` — override the scenario's `budget.duration` end-to-end (propagates into compile,
  control budget and the CLI timeout; e.g. `--budget 8h`).
- `--resume-on-finding` — after a `stop_on`-triggered finding, relaunch the run with the same scenario
  until `--budget` elapses.
- `--rotate-logs DIR` — tail every service's logs into `DIR/<svc>.log`, rotating at 50 MiB.

### `pull` flags

`--dest .confluence` (default), `--findings` (default true), `--corpus` (default false),
`--fuzz-service NAME` (auto-detected if empty).

## Scenario YAML

A Scenario declares the topology, workload, budget and oracles in one file. Validate it with
`confluence scenario validate <path>`, then `confluence up`/`run` it. Example —
`scenarios/soak-mixed-3x2.yaml`:

```yaml
apiVersion: confluence/v1
kind: Scenario
metadata:
  name: soak-mixed-3x2
  description: 3 rippled + 2 go-xrpl, soak workload, 10 min budget
topology:
  rippled:
    count: 3
    image: rippleci/rippled:2.6.2
  goxrpl:
    count: 2
    image: goxrpl:latest
workload:
  kind: soak            # soak | chaos | fuzz | replay | propagation | sync | consensus | ...
  tx_rate: 5
  accounts: 50
  rotate_every: 1000
  mutation_rate: 0.05
observability:
  enabled: false        # true → Prometheus + Grafana
budget:
  duration: 10m
  stop_on:
    - first_divergence
oracles:
  - state_diff
  - consensus_liveness
  - peer_health
```

| Section | Purpose |
| --- | --- |
| `metadata` | Human-readable name + description |
| `topology` | Per-implementation `count` and `image` |
| `workload` | The suite to run (`kind`) and its parameters |
| `observability` | Toggle the Prometheus + Grafana sidecars |
| `budget` | Wall-clock `duration` and `stop_on` predicates (e.g. `first_divergence`) |
| `oracles` | Which checks to run |

## Common workflows

### Validate, boot, watch

```bash
confluence scenario validate scenarios/soak-mixed-3x2.yaml
confluence up -f scenarios/soak-mixed-3x2.yaml
confluence status -w
```

### One-shot run (CI / fire-and-forget)

```bash
confluence run -f scenarios/soak-mixed-3x2.yaml
```

Exits when the budget elapses or a `stop_on` predicate fires.

### Overnight session — pin a branch, dashboard on, rotate logs, keep going

```bash
confluence run scenarios/soak-mixed-3x2.yaml \
    --rebuild-goxrpl /path/to/goXRPL-branch \
    --budget 8h \
    --with-dashboard \
    --rotate-logs ./logs \
    --resume-on-finding
```

### Inspect findings mid-run

```bash
confluence findings --limit 50
confluence findings --kind state_divergence
confluence finding show <id>
confluence events | jq 'select(.type=="finding")'
```

### Pull results, then tear down

```bash
confluence pull --corpus      # → .confluence/findings + .confluence/corpus
confluence down               # current enclave
```

## Troubleshooting

| Symptom | Fix |
| --- | --- |
| `kurtosis run` failed mid-startup | Enclave may be partially up — `confluence ls`, then `confluence down <name>` |
| `kurtosis boot watchdog tripped` | The boot hang fired and was auto-killed; the boot retries once. `--boot-hang-threshold 0` opts out (debug only) |
| `fuzz-soak service not found` on pull | Enclave torn down or sidecar never started — `confluence ls` first |
| Chaos actions silently no-op | docker-socket-proxy isn't running — `make docker-proxy` |
| Port 2375 already bound | Another proxy is up — `docker ps | grep docker-socket-proxy` and reuse or remove |
| Control health timeout on `up` | Bump `--wait-control 120s`, then `confluence logs -n control` |

## Next steps

- [Quickstart](/quickstart) — the legacy Makefile flow and `main.star` arguments.
- [Test Suites](/test-suites) — what each `workload.kind` does.
