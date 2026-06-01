---
description: What XRPL Confluence is, the problem it solves, and how the pieces fit together.
---

# Overview

<DownloadLLMsFullDoc />

XRPL Confluence is a [Kurtosis](https://www.kurtosis.com/) harness that orchestrates **mixed networks
of rippled and go-xrpl nodes** to validate that independent XRP Ledger implementations behave
identically. It exercises peer-to-peer messaging, transaction propagation, ledger sync, and consensus
compatibility, then drives the network with randomized, mutated, replayed and fault-injected workloads
to surface divergence.

## The problem

Two correct XRPL implementations must produce **byte-identical ledgers** from the same transactions.
A single mismatched `account_hash` or `transaction_hash` between rippled and go-xrpl is a consensus
fork waiting to happen on a real network. Unit tests and fixtures catch per-transaction bugs, but they
cannot catch:

- divergence that only appears after thousands of transactions of accumulated state,
- consensus stalls where a node silently stops advancing `validated_seq`,
- sync failures when a node joins a network that is already ahead,
- behaviour under network faults — latency, partitions, crashes and restarts.

Confluence targets exactly this gap: **differential testing of whole networks over time.**

## How it works

```
                ┌──────────────────────── Kurtosis enclave ───────────────────────┐
                │                                                                  │
                │   rippled-0   rippled-1   rippled-2 ...     go-xrpl-0  go-xrpl-1  │
                │      └───────────┴────── single private UNL ──────┴──────┘       │
                │                              ▲                                   │
                │            submit txs        │      poll ledger hashes           │
                │                              │                                   │
                │   ┌───────────┐    ┌─────────┴──────────┐    ┌────────────────┐  │
                │   │   fuzz    │───▶│  confluence-control │───▶│   dashboard    │  │
                │   │  sidecar  │    │  (oracles, findings)│    │  (live view)   │  │
                │   └─────┬─────┘    └─────────┬──────────┘    └────────────────┘  │
                │         │  corpus            │ findings store                    │
                └─────────┼────────────────────┼──────────────────────────────────┘
                          ▼                    ▼
                    /output/corpus     /var/confluence/findings
```

1. **Topology** ([`src/topology.star`](/topology)) generates validator keys and per-node config so all
   rippled and go-xrpl nodes form one private UNL (`NETWORK_ID = 10000`, quorum `ceil(0.8 × validators)`).
2. **Nodes** launch from `src/rippled/` and `src/goxrpl/` — each exposes peer (`51235`), RPC (`5005`)
   and WebSocket (`6006`) ports.
3. **The fuzz sidecar** submits transactions to the network and runs a two-layer differential oracle
   over the resulting ledgers (see [Sidecar & Oracle](/sidecar-oracle)).
4. **The control service** (`confluence-control`) exposes findings and SSE events, and runs the
   `consensus_stall` and `state_divergence` oracles for every run.
5. **The dashboard** renders a live view of every node's ledger, peers and logs.

## Components at a glance

| Component | Where | Role |
| --- | --- | --- |
| Topology generator | `src/topology.star` | Validator keys, per-node config, shared UNL |
| rippled / go-xrpl launchers | `src/rippled/`, `src/goxrpl/` | Bring up validator containers |
| Fuzz sidecar | `sidecar/` (`xrpl-confluence-sidecar:latest`) | Submit txs, run the differential oracle |
| Control service | `src/control_service.star` (`confluence-control`, `:8090`) | Findings API, SSE events, standing oracles |
| Dashboard | `src/dashboard/`, `dashboard/` (`:8080`) | Live network view |
| `confluence` CLI | `sidecar/cmd/confluence/` | Scenario-driven control surface |
| Scenarios | `scenarios/*.yaml` | Declarative topology + workload + budget |

## How it relates to the rest of the stack

Confluence is one of three interop layers:

- **Fixtures** — rippled-extracted test vectors that pin individual transaction outcomes.
- **Hive** — a multi-client orchestrator for conformance suites across rippled, go-xrpl and others.
- **Confluence** *(this project)* — a mixed-network fuzz / chaos / soak lab for whole-network
  differential testing.

Use fixtures for "does this transaction produce the right result?", and Confluence for "do these two
implementations stay in lockstep across a whole network, for hours, under stress?"

## Next steps

- [Quickstart](/quickstart) — get a network running.
- [Test Suites](/test-suites) — what each suite checks.
- [Topology & Control](/topology) — how the network is wired.
