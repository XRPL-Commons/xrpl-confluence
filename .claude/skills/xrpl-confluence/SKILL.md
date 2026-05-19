---
name: xrpl-confluence
description: Drive the xrpl-confluence fuzzing harness via the `confluence` CLI (or legacy Makefile). Use when the user wants to boot/tear down a Kurtosis enclave from a Scenario YAML, run a scenario end-to-end, list/inspect findings, stream logs/events, pull corpus, replay a reproducer, or run the older `make soak`/`make chaos` flows. Triggers on intents like "confluence up", "confluence run", "confluence findings", "list findings", "pull reproducer", "replay finding", "tail control events", "run a soak", "start chaos test", "tear down enclave".
---

# xrpl-confluence — fuzzing harness skill

`xrpl-confluence` orchestrates a multi-node XRPL test network (goXRPL + rippled) inside a Kurtosis enclave and runs a fuzz sidecar against it. There are **two CLI surfaces**:

1. **`confluence` CLI** (preferred) — Cobra binary at `sidecar/cmd/confluence/`. Scenario-driven. This is the canonical interface.
2. **`Makefile` recipes** (legacy) — `make soak` / `make chaos` flows kept for backward compatibility.

All commands run from the repo root: `/Users/thomashussenet/Documents/project_goXRPL/xrpl-confluence`.

## Building the CLI

The `confluence` binary isn't pre-installed. Build it on first use:

```bash
( cd sidecar && go build -o ../bin/confluence ./cmd/confluence )
export PATH="$PWD/bin:$PATH"   # or invoke as ./bin/confluence
confluence version
```

`bin/` is gitignored. Rebuild after pulling.

## Prerequisites

- `kurtosis` CLI installed and engine running (`kurtosis engine status`).
- Docker running locally — the sidecar runs as a container, scenarios pull/build node images.
- Go 1.25+ to build the CLI.
- For scenarios that drive container chaos (latency/restart/partition): the host-level docker-socket-proxy must be up. Start it with `make docker-proxy` (one-time per host).

If a prerequisite is missing, surface it and stop — don't auto-install Docker/Kurtosis.

## Global flags (work on every subcommand)

- `--enclave <name>` — target a specific enclave. Defaults to the current-context enclave when running a Scenario.
- `--control-url http://host:port` — bypass kurtosis lookup and hit the control service directly.
- `--json` — emit machine-readable NDJSON/JSON instead of human tables. Use this when piping into `jq` or chaining commands.

## Subcommand cheat sheet

| Command | Purpose |
| --- | --- |
| `confluence up -f scenarios/foo.yaml` | Boot an enclave from a Scenario YAML. |
| `confluence down [ENCLAVE]` | Tear down the current (or named) enclave. |
| `confluence ls` | List all confluence enclaves. |
| `confluence status` | Network status of the current enclave (nodes, peers, ledger). Add `-w` to refresh every 2 s. |
| `confluence run SCENARIO` | Boot + run + wait for budget/stop_on + optional tear-down. One-shot end-to-end. |
| `confluence replay REPRODUCER_ID` | Boot an enclave from a saved reproducer YAML. |
| `confluence logs -n NODE [-f] [--since 10m] [--grep regex]` | Stream a node's logs. |
| `confluence events` | Stream control-service SSE events as NDJSON (pipe to `jq`). |
| `confluence findings [--kind K] [--since ID] [--limit N]` | List findings from the running enclave. |
| `confluence finding show ID` | Show one finding in detail. |
| `confluence pull [--dest .confluence] [--corpus] [--no-findings]` | Mirror findings (and optionally corpus) from the enclave to the host. |
| `confluence scenario validate PATH` | Validate a Scenario YAML before booting. |

### `up` flags
`-f, --scenario PATH` (required), `--enclave NAME`, `--package DIR` (default `.`), `--tear-down-first` (default true), `--wait-control 60s`,
`--boot-hang-threshold 90s` (kill the kurtosis CLI if it stays silent past this — watchdog for the 1-in-3 0% CPU hangs; 0 disables),
`--rebuild-goxrpl PATH` (`docker build` PATH and tag it with `topology.goxrpl.image` before booting — so you don't need to remember `docker build -t goxrpl:latest <worktree>` separately),
`--rebuild-rippled PATH` (same idea, for `topology.rippled.image`),
`--with-dashboard` (force `observability.enabled=true` regardless of YAML — flips the grafana sidecar on without editing the scenario).

### `run` flags
All `up` flags above PLUS:
`-w, --wait` (default true), `--timeout DUR` (defaults to 2× scenario budget), `--down` (default true — tear down on finish),
`--budget DUR` (override the scenario's `budget.duration` end-to-end — propagates into compile, control budget, and the CLI timeout; e.g. `--budget 8h` for overnight),
`--resume-on-finding` (after a `stop_on`-triggered finding, relaunch the run with the same scenario until `--budget` elapses — for overnight fuzzing that wants more than one failure per session),
`--rotate-logs DIR` (tail every service's kurtosis logs into `DIR/<svc>.log`, rotating at 50 MiB — survives overnight when the in-container ring buffer is too small for a post-mortem).

### `pull` flags
`--dest .confluence` (default), `--findings` (default true), `--corpus` (default false), `--fuzz-service NAME` (auto-detect if empty).

## Common workflows

### Validate then boot a scenario
```bash
confluence scenario validate scenarios/soak-mixed-3x2.yaml
confluence up -f scenarios/soak-mixed-3x2.yaml
confluence status -w
```

### One-shot run (boot, wait for budget, tear down)
```bash
confluence run -f scenarios/soak-mixed-3x2.yaml
```
Useful for CI or "fire and forget" — exits when the scenario's budget elapses or a `stop_on` predicate fires.

### Overnight session (rebuild from a worktree, dashboard on, log rotation, keep going through findings)
```bash
confluence run scenarios/soak-mixed-3x2.yaml \
    --rebuild-goxrpl /path/to/goXRPL-branch \
    --budget 8h \
    --with-dashboard \
    --rotate-logs ./logs \
    --resume-on-finding
```
One command pins "this branch, this commit, rebuild if needed" — forgetting it used to silently test stale code.

### Inspect findings while a run is in flight
```bash
confluence findings --limit 50
confluence findings --kind state_divergence
confluence findings --kind consensus_stall
confluence finding show <id>
confluence events | jq 'select(.type=="finding")'
```

Two oracles run automatically in the control service for every `confluence up` (no scenario opt-in needed):

- **`consensus_stall`** — fires when any node's `closed_seq - validated_seq` exceeds 10 for ≥ 2 min (tunable via `--stall-gap-threshold` / `--stall-sustain` on `confluence-control`). Catches the "network silently broken, validated_seq frozen forever" case that `state_diff` alone misses because `state_diff` only ticks when `validated_seq` advances.
- **`state_divergence`** — when two nodes report different ledger hashes at the same seq, the finding's `Detail` now embeds a `LedgerDiff` snapshot of every node's ledger at that seq: per-node root hashes, common tx hashes, `only_on_nodes`, and `suspect_tx_types` (union of `TransactionType` from any tx not on every node). `SuspectedComponents` mirrors `suspect_tx_types` for quick triage. Use `confluence finding show <id>` (or `jq '.detail'` on `confluence pull --findings`) to see the full diff — no need to grep container logs or hand-write a replay script.

### Mirror findings + corpus to the host
```bash
confluence pull --corpus              # → .confluence/findings + .confluence/corpus
```
`.confluence/` is per-machine state (gitignored).

### Replay a reproducer
```bash
confluence replay <reproducer-id>
```

### Tear down
```bash
confluence down                       # current enclave
confluence down xrpl-soak             # named
```

## Legacy Makefile flow (soak / chaos)

The Makefile predates the CLI. Use it only when explicitly asked.

```bash
make soak                                # 2 goXRPL + 3 rippled, tx_rate=5
make soak GOXRPL_COUNT=3 RIPPLED_COUNT=5 TX_RATE=10 OBSERVABILITY=1
make soak-status / soak-tail / soak-pull / soak-down

make docker-proxy                        # required for chaos (one-time)
make chaos                               # reads .chaos-schedule.json
make chaos-status / chaos-tail / chaos-pull / chaos-down
```

Tunables (Make vars): `ENCLAVE`, `GOXRPL_COUNT`, `RIPPLED_COUNT`, `TX_RATE`, `ACCOUNTS`, `ROTATE_EVERY`, `MUTATION_RATE`, `CORPUS`, `OBSERVABILITY`, `ALERT_WEBHOOK_URL`, `CHAOS_SCHEDULE`.

## Failure-recovery patterns

- **`kurtosis run` failed mid-startup** → enclave may be partially up. `confluence ls` to inspect, then `confluence down <name>` (or `make soak-down`).
- **`kurtosis boot watchdog tripped after Ns of silence`** → the 0% CPU hang fired and was auto-killed; `up`/`run` retries the boot once with a fresh enclave. If both attempts trip, surface the message and stop — `--boot-hang-threshold 0` opts out entirely (don't use unless debugging the watchdog itself).
- **`fuzz-soak service not found` on pull** → enclave torn down or sidecar never started. `confluence ls` first.
- **Chaos container actions silently no-op** → docker-socket-proxy isn't running. `make docker-proxy`.
- **Port 2375 already bound** → another proxy is up; `docker ps | grep docker-socket-proxy` and reuse or remove.
- **Control service health timeout on `up`** → bump `--wait-control 120s`, then check `confluence logs -n control` (or via `kurtosis service logs`).
- **Forgetting `docker build` before `up`** → don't `docker build` separately; pass `--rebuild-goxrpl <path>` / `--rebuild-rippled <path>` to `up`/`run` so the image kurtosis pulls is byte-for-byte the one just built.

## Files worth reading before non-trivial changes

- `sidecar/cmd/confluence/` — CLI source of truth (one file per subcommand).
- `sidecar/internal/api/` — control-service contract used by the CLI.
- `Makefile` — legacy CLI surface.
- `main.star` / `src/topology.star` — Kurtosis enclave topology.
- `scenarios/*.yaml` — declarative scenarios consumed by `up` / `run`.
- `docs/plans/` — milestone design docs (chaos-runner, etc.).

## Output discipline

When booting a test the user usually wants:
1. Confirmation it started + the enclave name.
2. The dashboard URL if observability is on.
3. The next command to inspect or pull results.

Never tail logs or `events` in the foreground unless asked — they don't return. Use `run_in_background` and report once a pattern of interest is hit. Prefer `--json` output when chaining commands.
