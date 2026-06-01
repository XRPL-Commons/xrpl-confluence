---
description: The fuzz sidecar, the two-layer differential oracle, and how findings are stored.
---

# Sidecar & Oracle

<DownloadLLMsFullDoc />

The **fuzz sidecar** (`xrpl-confluence-sidecar:latest`, built from `sidecar/`) is the active component:
it submits transactions to the network and runs the **differential oracle** that decides whether the
implementations agree. The same image runs in four modes.

## Modes

| Mode | Bounded? | Service | Purpose |
| --- | --- | --- | --- |
| `fuzz` | yes (`tx_count`) | one-shot | Bounded randomized run |
| `replay` | yes (ledger range) | one-shot | Replay mainnet ledgers |
| `soak` | no | `fuzz-soak` | Unbounded driver |
| `chaos` | no | `fuzz-chaos` | Unbounded driver + scheduled faults |
| `shrink` | yes (prefix) | one-shot | Minimal-reproducer prefix replay |

The sidecar exposes a `results` port on `8081` with a `/status` endpoint, and a `/metrics` endpoint
for Prometheus when observability is enabled.

### Key environment variables

| Var | Meaning |
| --- | --- |
| `MODE` | `fuzz` · `replay` · `soak` · `chaos` · `shrink` |
| `NODES` | Comma-separated node HTTP URLs to poll |
| `SUBMIT_URL` | Node to submit transactions to |
| `ACCOUNTS` | Number of test accounts |
| `BATCH_CLOSE` | Interval between layer-1 batch oracle checks (default `5s`) |
| `CORPUS_DIR` | `/output/corpus` |
| `FINDINGS_MIRROR_DIR` | `/var/confluence/findings` (soak/chaos — shared with the control service) |
| `DOCKER_HOST` | `tcp://host.docker.internal:2375` — crash detection via the socket-proxy |
| `CRASH_LABEL_KEY` / `CRASH_LABEL_VAL` | Container label identifying nodes (`fuzzer.role=node`) |

Mode-specific vars layer on top: `TX_COUNT` (fuzz); `TX_RATE` / `ROTATE_EVERY` / `MUTATION_RATE`
(soak, chaos); `CHAOS_SCHEDULE` (chaos); `MAINNET_URL` / `REPLAY_LEDGER_START` /
`REPLAY_LEDGER_END` (replay); `SHRINK_LOG` / `SHRINK_DIVERGENCE` / `SHRINK_MAX_STEP` (shrink).

## The differential oracle

The oracle works at two layers:

### Layer 1 — batch ledger-hash oracle
Every `BATCH_CLOSE` interval the sidecar queries each node's ledger at a common sequence and compares
the structural hashes — `ledger_hash`, `account_hash`, and `transaction_hash`. A mismatch at the same
sequence is a **state divergence**: two implementations built a different ledger from the same input.

### Layer 2 — per-transaction oracle
As transactions are submitted, the sidecar cross-checks each node's response and per-tx effects, so a
divergence can be attributed to the specific transaction(s) that caused it rather than just "sometime
in the last batch."

### Standing oracles in the control service
In addition to the sidecar, `confluence-control` runs `consensus_stall` and `state_divergence` for
every run (see [Topology & Control](/topology)).

## Findings

When the oracle detects a problem it writes a **finding**. A `state_divergence` finding embeds a
`LedgerDiff` snapshot of every node's ledger at the diverging sequence:

- per-node root hashes,
- the set of common transaction hashes,
- `only_on_nodes` — transactions present on some nodes but not others,
- `suspect_tx_types` — the union of `TransactionType`s for any transaction not on every node, mirrored
  into `SuspectedComponents` for quick triage.

This means you can diagnose a divergence without grepping container logs or hand-writing a replay
script — `confluence finding show <id>` (or `jq '.detail'` over a pulled finding) shows the full diff.

### Findings store
Findings are mirrored from the sidecar's `FINDINGS_MIRROR_DIR` into the shared `confluence-findings`
volume at `/var/confluence/findings`, where the control service's disk-watcher picks them up and serves
them over the API. Inspect them with:

```bash
confluence findings --limit 50
confluence findings --kind state_divergence
confluence findings --kind consensus_stall
confluence finding show <id>
confluence events | jq 'select(.type=="finding")'
```

Pull findings (and optionally the corpus) to the host with:

```bash
confluence pull --corpus      # → .confluence/findings + .confluence/corpus
```

`.confluence/` is per-machine state and is gitignored.

## Corpus, replay & shrink

Submitted transactions accumulate in the corpus at `/output/corpus`, persisted in the
`fuzz-soak-output` / `fuzz-chaos-output` volumes. For week-long runs, `scripts/corpus-pull-loop.sh`
(or `make soak-pull-loop`) snapshots the corpus to the host on an interval without stopping the
enclave.

A captured divergence feeds the **shrink** workflow ([Test Suites](/test-suites#shrink)): the run log
and divergence are replayed as a prefix, and `scripts/shrink.sh` binary-searches the step cap to find
the minimal failing transaction sequence.

## Crash detection

The sidecar polls Docker (via the socket-proxy at `DOCKER_HOST`) for node containers labelled
`fuzzer.role=node`. If a node crashes, it captures the tail of its logs (`CRASH_TAIL_LINES`) as part
of the finding — so a crash and the transactions leading up to it are recorded together.

## Optional observability

When `enable_observability` (scenario / `OBSERVABILITY=1`) is set, a Prometheus sidecar scrapes the
fuzz `/metrics` endpoint and a Grafana instance (anonymous viewer, port `3000`) renders it from
`dashboard/grafana-provisioning/`. The CLI's `--with-dashboard` flag forces this on regardless of the
scenario.

## Next steps

- [Chaos](/chaos) — drive the sidecar's chaos mode.
- [Dashboard](/dashboard) — the live network view.
