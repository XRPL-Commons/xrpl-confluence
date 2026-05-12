# Agent-Friendly Confluence Interface — Design

**Date:** 2026-05-12
**Status:** Design — pending implementation plan

## Goal

Make xrpl-confluence a first-class tool for AI agents working on goXRPL (and other XRPL implementations) without compromising human ergonomics. Today the harness is driven by `make` targets, colored shell output, a browser dashboard, and ad-hoc `docker cp` for artifacts — all human-shaped. This design promotes the harness to a **typed, machine-driven control surface** with a CLI that is equally useful at a terminal and inside an agent loop.

## Non-Goals

- Shipping an MCP server. MCP is a thin wrapper over the contract this design establishes; it gets its own follow-up spec.
- Auth, RBAC, multi-tenant operation. Single-developer-machine assumption.
- Replacing Starlark. Kurtosis remains the bootstrap mechanism; scenarios *compile to* Kurtosis args.
- Performance benchmarking or new test workloads. This is an interface change, not a coverage change.

## Approach

Build a Go `confluence` CLI on top of the existing `sidecar/`, and promote the sidecar from a private fuzz/oracle backend into a **long-lived control service** exposing a stable HTTP+JSON API. The CLI is a thin client over that API. The dashboard becomes a second client of the same API. MCP, when it comes, becomes a third.

Chosen over an MCP-first approach (locks design to one consumer; humans/CI lose ergonomics) and over a REST/OpenAPI codegen approach (heaviest first move; OpenAPI-generated MCP tools tend to be poorly shaped for agents).

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  Host (developer machine / CI)                                  │
│                                                                 │
│  ┌──────────────────┐         kurtosis CLI                      │
│  │  confluence CLI  │────────► kurtosis run … (still the boot)  │
│  │  (Go binary)     │                                           │
│  │                  │   HTTP/JSON + SSE                         │
│  │  --json on every │◄─────────────────────┐                    │
│  │  subcommand      │                      │                    │
│  └──────────────────┘                      │                    │
│         ▲                                  │                    │
│         │ same Go packages                 │                    │
│         ▼                                  │                    │
│  ┌──────────────────┐                      │                    │
│  │  sidecar/  (lib) │                      │                    │
│  └──────────────────┘                      │                    │
└────────────────────────────────────────────┼────────────────────┘
                                             │
┌────────────────────────────────────────────┼────────────────────┐
│  Kurtosis enclave                          │                    │
│                                            │                    │
│  ┌──────────────────────────────────────┐  │                    │
│  │  confluence-control service          │◄─┘                    │
│  │  (long-lived; promoted sidecar)      │                       │
│  │  - /v1/scenarios, /v1/runs,          │                       │
│  │    /v1/findings, /v1/nodes,          │                       │
│  │    /v1/state/diff, /v1/logs,         │                       │
│  │    /v1/events (SSE)                  │                       │
│  └──────────┬───────────────────────────┘                       │
│             │  reads / drives                                   │
│  ┌──────────▼────────┐  ┌─────────────┐  ┌──────────────┐       │
│  │  rippled × N      │  │ goxrpld × M │  │  fuzz / chaos │       │
│  └───────────────────┘  └─────────────┘  └──────────────┘       │
│                                                                 │
│  ┌──────────────────────────────────────┐                       │
│  │  dashboard (static + WS)             │  ← becomes a client   │
│  │   talks to confluence-control        │     of the same API    │
│  └──────────────────────────────────────┘                       │
└─────────────────────────────────────────────────────────────────┘
```

### Component boundaries

All new Go code lives inside the existing `sidecar/` Go module (`sidecar/go.mod`). The module name "sidecar" is now misleading — it hosts both an in-enclave control service and a host-side CLI — but renaming is a mechanical follow-up, not part of this spec.

- `sidecar/cmd/confluence/` — Cobra CLI binary. Subcommands are thin: parse flags → call client → format (table for humans, JSON for `--json`).
- `sidecar/cmd/confluence-control/` — control-service binary, run in the enclave alongside existing workload binaries (e.g. `cmd/fuzz/`). The workload binaries report into it; it owns the API and finding store.
- `sidecar/internal/client/` — typed Go client for the control API. Shared by CLI and (later) MCP.
- `sidecar/internal/server/` — HTTP handlers for `/v1`. Imported by `cmd/confluence-control`.
- `sidecar/internal/api/` — request/response types + JSON schemas. Single source of truth for both ends and for generated dashboard types.
- `sidecar/internal/scenario/` — load/validate/compile YAML scenarios into Kurtosis args and runtime config. The **only** producer of Kurtosis `args` JSON; the Makefile shell-quoted JSON in today's `chaos:` target is retired.
- `sidecar/internal/finding/` — finding types, detectors, store (filesystem-backed inside the enclave, mirrored to host via `confluence pull`).
- `sidecar/internal/{fuzz,oracle,rpcclient}/` — existing packages, unchanged. The new `finding/` and `server/` packages consume them.

### Why a long-lived control service, not a one-shot CLI

`confluence diff`, `confluence findings`, `confluence logs --since`, and `confluence events` all need a process that has been observing the network since boot. The sidecar is already that process today (it powers the dashboard); this design promotes it to a public API instead of a private backend.

## CLI surface

Top-level: `confluence <command> [flags]`. Every subcommand accepts `--json`. Exit codes:

- `0` success
- `1` user error (bad flags, scenario invalid)
- `2` confluence/control-service error (transport, internal)
- `3` command succeeded but findings were opened — lets CI gate on `$?` while agents inspect the JSON `findings` array

| Command | Purpose | Output shape |
|---|---|---|
| `confluence up [--scenario FILE\|NAME]` | Boot an enclave from a scenario. Wraps `kurtosis run`. | `{enclave_id, control_url, nodes[]}` |
| `confluence down [ENCLAVE]` | Tear down. | `{enclave_id, ok}` |
| `confluence ls` | List enclaves + scenarios + health. | `[{enclave_id, scenario, started_at, status, findings_count}]` |
| `confluence status [--watch]` | Snapshot of network health: per-node ledger index, peers, sync state, divergence summary. | `{nodes[], divergence, consensus, last_finding}` |
| `confluence diff [--at-ledger N]` | State divergence between goXRPL and rippled at a ledger. | `{ledger, divergent_keys[], suspected_components[]}` |
| `confluence findings [--since ID] [--kind ...]` | List findings. | `[{id, kind, summary, reproducer_id, opened_at, ...}]` |
| `confluence finding show ID` | Full finding. | `{...finding, log_excerpt, reproducer_path}` |
| `confluence replay REPRODUCER_ID` | Fresh enclave that re-runs only the offending workload. Sugar for `confluence run .confluence/reproducers/<id>.yaml`. | `{enclave_id, run_id, finding_ids[]}` |
| `confluence logs --node NAME [--since DUR] [--grep RE]` | Slice logs. | NDJSON one record per line |
| `confluence events [--since CURSOR]` | Stream control events (node up/down, ledger close, finding opened). | SSE / NDJSON |
| `confluence run SCENARIO [--wait] [--timeout]` | Boot + run + wait until termination/timeout; emit final report. | `{run_id, status, findings[], duration_ms}` |
| `confluence scenario {list,show,validate,init}` | Manage scenario files. | scenario JSON |
| `confluence pull` | Copy findings + corpus from enclave to host (replaces `make soak-pull`). | `{path, count}` |

`confluence run` is the **agent loop primitive**: one call, blocks, returns structured pass/fail with reproducers. Every other command exists for drilling in.

### Discovery

`confluence up` writes `.confluence/current.json` to the CWD: `{enclave_id, control_url, started_at}`. All other commands read it unless `--enclave` is passed. Mirrors `kubectl`/`docker context` ergonomics and removes the "find the dashboard IP" dance.

## Scenarios

YAML files under `scenarios/` consolidate what is today split across `main.star` args, Makefile vars, and `.chaos-schedule.json`.

```yaml
# scenarios/soak-mixed-3x2.yaml
apiVersion: confluence/v1
kind: Scenario
metadata:
  name: soak-mixed-3x2
  description: 3 rippled + 2 goXRPL, soak workload, 10 min budget
topology:
  rippled: { count: 3, image: rippleci/rippled:2.6.2 }
  goxrpl:  { count: 2, image: goxrpl:latest }
workload:
  kind: soak           # soak | fuzz | replay | shrink | none
  tx_rate: 5
  accounts: 50
  rotate_every: 1000
  mutation_rate: 0.05
chaos:
  schedule: []         # same shape as today's .chaos-schedule.json entries
observability:
  enabled: false
budget:
  duration: 10m
  stop_on:
    - first_divergence # | first_crash | none
oracles:
  - state_diff
  - consensus_liveness
  - peer_health
```

Rules:

- `confluence scenario validate FILE` checks shape + cross-field constraints (e.g. `workload.kind: replay` requires `reproducer.id`). Returns JSON errors with field paths.
- `confluence scenario init` writes a commented starter per `kind`.
- `apiVersion: confluence/v1` is frozen once shipped. Breaking changes → `v2`.
- Built-in scenarios ship under `scenarios/`; user scenarios are picked up from `./scenarios/` and `$CONFLUENCE_SCENARIOS`.
- Budget is mandatory. No infinite runs from the CLI; long-lived enclaves are a dashboard concern.
- Oracles are opt-in additive — `[]` means "boot the network, don't judge it".

## Findings

A finding is a typed, durable record of something the network did wrong. This is the artifact agents read; everything else exists to populate or contextualize findings.

### Shape (`internal/api/finding.go`)

```json
{
  "id": "fnd_01HXYZ...",
  "run_id": "run_01HXYW...",
  "enclave_id": "xrpl-soak",
  "scenario": "soak-mixed-3x2",
  "kind": "state_divergence",
  "severity": "error",
  "opened_at": "2026-05-12T14:03:21Z",
  "closed_at": null,
  "summary": "goxrpl-1 disagrees with rippled-0 on AccountRoot rXYZ at ledger 1423",
  "detail": { },
  "evidence": {
    "log_excerpts": [
      { "node": "goxrpl-1", "since": "...", "lines": ["..."] }
    ],
    "ledger_range": [1420, 1424],
    "diff_keys": ["00...AccountRoot:rXYZ"]
  },
  "reproducer": {
    "id": "rpr_01HXYV...",
    "scenario_path": ".confluence/reproducers/rpr_01HXYV.yaml",
    "kind": "replay"
  },
  "suspected_components": ["tx/payment", "ledger/state"]
}
```

### Kinds (v1, closed set)

| kind | Detected by | Closes when |
|---|---|---|
| `state_divergence` | `state_diff` oracle | nodes re-converge at later ledger |
| `consensus_stall` | `consensus_liveness` oracle | network advances |
| `peer_drop` | `peer_health` oracle | peer reconnects within grace |
| `node_crash` | container exited non-zero | never (terminal) |
| `fuzz_failure` | fuzz workload reports rejection / mismatch | never |
| `chaos_violation` | invariant breached during a chaos event | depends on invariant |

New kinds in subsequent versions require an `apiVersion` bump or are emitted as `kind: "unknown"` so old clients don't break.

### Storage & retrieval

- Control service exposes `/v1/findings`, `/v1/findings/{id}`, `?since=<id>` for incremental tail.
- Persisted to `/var/confluence/findings/*.json` inside the enclave; `confluence pull` mirrors to `./.confluence/findings/` and reproducer YAMLs to `./.confluence/reproducers/`.
- Reproducers are scenarios (`workload.kind: replay`). No special replay storage format. `confluence replay ID` is sugar for `confluence run .confluence/reproducers/<id>.yaml`.

### YAGNI cuts (locked in for v1)

- No dedup/grouping. Two identical divergences = two findings. Grouping is a UI concern.
- No comments/triage/assignees. This is not a bug tracker.
- `suspected_components` is best-effort, not a contract. Empty is valid.

## Control API & error model

HTTP+JSON on the control service, versioned at `/v1`. SSE for streaming. The CLI's typed Go client is the reference consumer; the dashboard becomes the second consumer.

| Method | Path | Notes |
|---|---|---|
| GET | `/v1/scenarios` | list built-ins |
| POST | `/v1/scenarios/validate` | server-side validation so rules don't drift between CLI and server |
| GET | `/v1/runs`, `/v1/runs/{id}` | run history, current run |
| POST | `/v1/runs` | start a run (body: compiled scenario) |
| GET | `/v1/nodes`, `/v1/nodes/{name}` | per-node status |
| GET | `/v1/state/diff?at=N` | divergence snapshot |
| GET | `/v1/findings?since=ID&kind=...` | list/tail |
| GET | `/v1/findings/{id}` | full record |
| GET | `/v1/logs?node=…&since=…&grep=…&follow=0/1` | NDJSON |
| GET | `/v1/events?since=CURSOR` | SSE: node, ledger, finding events |
| GET | `/v1/healthz` | liveness probe + CLI `--wait` |

### Error envelope

All non-2xx responses, mirrored in CLI `--json` stderr:

```json
{
  "error": {
    "code": "scenario_invalid",
    "message": "workload.kind=replay requires reproducer.id",
    "field": "workload.reproducer.id",
    "hint": "set reproducer.id or change workload.kind"
  }
}
```

Codes are a closed set per endpoint (documented in `internal/api/`). The CLI maps `code` → exit code so agents/CI don't grep messages.

### Versioning

`/v1` is frozen once shipped. Additive fields fine; removals/renames go to `/v2`. CLI sends `X-Confluence-Client: confluence/<version>`; server warns on mismatch in `--json` output.

## Repository layout

```
xrpl-confluence/
  sidecar/                          # existing Go module — extended, not split
    go.mod                          # unchanged
    cmd/
      fuzz/                         # existing — unchanged
      confluence/                   # NEW: Cobra CLI binary (host)
      confluence-control/           # NEW: control-service binary (in-enclave)
    internal/
      fuzz/        oracle/        rpcclient/   # existing — unchanged
      api/                          # NEW: shared types + JSON schemas
      client/                       # NEW: typed Go client over /v1
      server/                       # NEW: HTTP handlers for /v1
      scenario/                     # NEW: YAML load/validate/compile-to-kurtosis-args
      finding/                      # NEW: types, store, detectors
  scenarios/                        # NEW: built-in v1 scenarios (soak, chaos, replay templates)
  dashboard/                        # existing — becomes a /v1 client; TS types generated from sidecar/internal/api/
  docs/design/2026-05-12-agent-friendly-interface-design.md  # this doc
  agents.md                         # NEW: how an AI agent should drive confluence
```

The Makefile shrinks: it shells out to `confluence` for everything except the developer aliases (`make soak` becomes `confluence run scenarios/soak-mixed-3x2.yaml --wait`). Repo-level convenience: `make build` produces `./bin/confluence` from `./sidecar/cmd/confluence` so contributors don't have to know it lives under `sidecar/`.

## Testing strategy

Three layers, mapped to the three components:

1. **Unit (Go).** Scenario load/validate/compile, finding store, oracle detectors, error envelope. No Docker. Fast. Lives next to source.
2. **Control-service integration (Go, no Kurtosis).** HTTP server in-process with fake node clients; asserts API contract — status codes, error codes, JSON shape, SSE framing. Golden JSON files under `internal/server/testdata/`. This is where the agent-facing contract is locked down.
3. **End-to-end (existing Kurtosis harness).** One e2e per critical CLI verb: `up`, `run --wait`, `findings`, `replay`. Driven through the CLI, not Starlark. Gated behind `CONFLUENCE_E2E=1` so contributors without Docker can still run units.

**Contract test for dashboard:** dashboard TypeScript types are generated from `sidecar/internal/api/` via a small `go run ./sidecar/cmd/api-gen` step. A unit test fails CI if generated types drift from committed ones. Same generator later produces MCP tool schemas.

## Agent ergonomics: `agents.md`

A repo-root `agents.md` tells AI tools (Claude Code, Cursor, Codex, etc.) the minimum to drive confluence:

- The agent loop: `confluence run <scenario> --wait --json` → parse `findings[]` → `confluence finding show <id> --json` → fix code → `confluence replay <reproducer_id> --json`.
- The closed set of `error.code` and `finding.kind` values, with one-line semantics for each.
- The location of `.confluence/current.json`, findings, reproducers on the host.
- A pointer to `confluence scenario init` for authoring new scenarios.

This is the human-readable counterpart of the typed API: enough context to make the JSON contract self-driving.

## Open questions

None blocking. Items deferred to follow-up specs:

- MCP server wrapping `/v1`.
- Multi-enclave / multi-developer operation (auth, routing).
- Finding grouping/dedup in the dashboard.
- A `confluence inject` ad-hoc chaos command (additive, doesn't change the contract).

## Migration

Existing Makefile targets keep working through a one-release deprecation window — each delegates to `confluence` under the hood and prints a deprecation note. The `.chaos-schedule.json` shell-quoting path is the first thing to go; chaos schedules move into the scenario `chaos.schedule` field on day one.
