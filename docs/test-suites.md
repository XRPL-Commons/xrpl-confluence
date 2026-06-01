---
description: The test suites Confluence runs against a mixed network and what each one asserts.
---

# Test Suites

<DownloadLLMsFullDoc />

A suite is selected with `test_suite` (via `main.star`, the `confluence` CLI, or a Scenario
`workload.kind`). Suites fall into two families:

- **Bounded smoke / replay suites** run inline and finish on their own: `propagation`, `sync`,
  `consensus`, `delayed_sync`, `fuzz`, `replay`, `shrink`.
- **Unbounded driver suites** stay up until you tear them down: `soak`, `chaos`.

`test_suite: "all"` runs the bounded smoke suites in sequence.

## Minimum topology per suite

`main.star` validates the topology before booting anything:

| Suite | Min rippled | Min go-xrpl |
| --- | --- | --- |
| `soak`, `chaos`, `shrink` | 2 | 1 |
| `fuzz`, `replay` | 2 | 0 |
| *(everything else)* | — | total ≥ 2 |

## Smoke suites

### `propagation`
Asserts that transactions cross implementation boundaries: a transaction submitted to rippled reaches
go-xrpl, and one submitted to go-xrpl reaches rippled. The network is advanced to a few closed
ledgers, test transactions are submitted, and receipt on the other implementation is verified.

### `sync`
Asserts that a **late-joining go-xrpl node** can sync from a network that has already advanced. The
existing network builds up funded-account state, a new go-xrpl node is launched, and its catch-up to
the network tip is verified.

### `consensus`
Asserts that a **mixed validator set** (rippled + go-xrpl) reaches consensus and that all nodes agree
on the validated ledger hash. Nodes are advanced past several closed ledgers and their `server_info`
and validated ledger hashes are compared.

### `delayed_sync`
A staggered-startup variant: rippled launches first and advances, the dashboard starts rippled-only,
then go-xrpl joins and is observed syncing into the live network. Useful for reproducing late-join
behaviour.

## Driver suites

### `soak`
The workhorse. After the network reaches a few closed ledgers, the `fuzz-soak` sidecar drives an
**unbounded** transaction stream while the differential oracle runs. The enclave stays up until you
tear it down.

`soak_args`:

| Arg | Default | Meaning |
| --- | --- | --- |
| `tx_rate` | `0` (unlimited) | Transactions per second |
| `rotate_every` | `1000` | Rotate the active account tier every N txs |
| `mutation_rate` | `0.0` | Fraction `[0,1]` of txs to mutate before submitting |
| `accounts` | `50` | Number of test accounts |
| `enable_observability` | `false` | Launch Prometheus + Grafana |
| `alert_webhook_url` | — | Optional alert webhook |

Corpus accumulates in the `fuzz-soak-output` volume at `/output/corpus`; extract it with
`confluence pull --corpus` or `make soak-pull`.

### `chaos`
A soak loop plus a **scheduled fault-injection** track (latency, restarts, partitions). The go-xrpl
image is swapped for `goxrpl-tools:latest` so `iproute2`/`iptables` are available inside containers,
and container actions require the host docker-socket-proxy. See [Chaos](/chaos) for the schedule
format and setup.

## Replay & shrink

### `fuzz`
A **bounded** run of `tx_count` transactions (default `100`, `accounts` default `10`, optional `seed`
for reproducibility). Waits for completion and reports results from the sidecar's `/status` endpoint
on port `8081`.

### `replay`
Fetches a range of **mainnet ledgers** over JSON-RPC and replays their transactions against the test
topology.

| Arg | Default | Meaning |
| --- | --- | --- |
| `mainnet_url` | `https://s1.ripple.com:51234` | Public rippled endpoint |
| `ledger_start` | `80000000` | First ledger to replay (inclusive) |
| `ledger_end` | `80000005` | Last ledger to replay (inclusive) |
| `accounts` | `10` | Number of test accounts |
| `seed` | — | Optional, for reproducibility |

### `shrink`
Replays a **prefix** of a saved fuzz run to find the minimal transaction sequence that reproduces a
divergence. It takes a Kurtosis artifact (`shrink_artifact`) containing `run.ndjson` + `div.json` and
an inclusive step cap (`shrink_max_step`), then reports whether the divergence reproduced. The
`scripts/shrink.sh` helper binary-searches the step cap to converge on the smallest failing prefix:

```bash
scripts/shrink.sh <run-log.ndjson> <divergence.json> [seed]
```

`accounts`, `seed` and `validate_timeout` must match the original run for the replay to be
deterministic.

## Next steps

- [Sidecar & Oracle](/sidecar-oracle) — how findings are produced.
- [Chaos](/chaos) — fault injection in depth.
- [CLI & Scenarios](/cli) — drive suites declaratively.
