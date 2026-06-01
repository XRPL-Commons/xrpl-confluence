---
description: Fault injection — latency, restarts and partitions — and the chaos schedule format.
---

# Chaos

<DownloadLLMsFullDoc />

The **chaos** suite runs a soak workload *and* injects faults on a schedule: container restarts,
network latency, and partitions. It is how Confluence checks that the network not only agrees under
load, but recovers correctly when the underlying infrastructure misbehaves.

## Prerequisites

Chaos needs Docker-level control of the node containers, which the Kurtosis enclave cannot do directly.
Two things are set up for you, but the proxy is a one-time host step:

1. **docker-socket-proxy** — start it once per host:

   ```bash
   make docker-proxy
   ```

   This runs `tecnativa/docker-socket-proxy` exposing Docker on `tcp://host.docker.internal:2375`. The
   fuzz sidecar dials it to restart containers (`CONTAINERS=1`), run `netem`/partition commands inside
   them (`EXEC=1`), and check liveness (`PING=1`). Stop it with `make docker-proxy-down`.

2. **`goxrpl-tools:latest`** — the chaos suite automatically swaps the go-xrpl image for this
   `iproute2`/`iptables`-equipped variant so latency and partition actions work. Build it with
   `scripts/build-goxrpl-tools.sh` (needs `goxrpl:latest` first).

## Running chaos

### Via the Makefile

```bash
make docker-proxy                          # one-time per host
cp .chaos-schedule.example.json .chaos-schedule.json
# edit the schedule to taste
make chaos                                 # reads .chaos-schedule.json
make chaos-tail                            # stream fuzz-chaos logs
make chaos-pull                            # copy /output/corpus → ./.chaos-corpus
make chaos-down                            # tear down
```

### Via the CLI / Scenario

Use a Scenario with `workload.kind: chaos` and a `schedule`, then `confluence up`/`run`. The schedule
is passed through to the sidecar as `chaos_args.schedule` (a JSON string).

## The schedule format

The schedule is a **JSON array of events**. There are two shapes.

### One-shot events
Fire once at a transaction `step`, then recover after `recover_after` seconds:

```json
{
  "step": 50,
  "type": "restart",
  "container": "rippled-1",
  "recover_after": 25
}
```

### Recurring events
Repeat on a wall-clock interval until a step cap, with optional jitter:

```json
{
  "type": "recurring",
  "recurring": {
    "every": 600,
    "until_step": 12000,
    "jitter": 30,
    "event": {
      "type": "latency",
      "container": "rippled-*",
      "iface": "eth0",
      "delay_ms_min": 50,
      "delay_ms_max": 500,
      "recover_after": 120
    }
  }
}
```

### Fault types

| `type` | Effect | Fields |
| --- | --- | --- |
| `restart` | Kill and restart a container | `container`, `recover_after` |
| `latency` | Add network delay via `netem` | `container`, `iface` (e.g. `eth0`), `delay_ms_min`, `delay_ms_max`, `recover_after` |
| `partition` | Network-partition a container | `container`, `recover_after` |

### Container selectors

- Exact: `"rippled-1"`, `"goxrpl-0"` — target a single container.
- Wildcard: `"rippled-*"`, `"goxrpl-*"` — target all containers of that type.

The complete example lives in `.chaos-schedule.example.json` at the repo root.

## What it proves

While faults fire, the differential oracle keeps running. Chaos surfaces failures that only appear
under disruption:

- a node that **forks** after a restart instead of resyncing,
- a network that **stalls** (`consensus_stall`) and never recovers after a partition heals,
- a **state divergence** that only manifests when messages are delayed or reordered.

Findings are captured exactly as in soak mode (see [Sidecar & Oracle](/sidecar-oracle)), with the
crash-log tail attached when a node goes down. The corpus persists to `fuzz-chaos-output`, so any
divergence can be fed into the [shrink](/test-suites#shrink) workflow.

## Next steps

- [Sidecar & Oracle](/sidecar-oracle) — how chaos findings are detected and stored.
- [Test Suites](/test-suites) — the other suites.
