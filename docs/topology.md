---
description: How Confluence wires a mixed rippled + go-xrpl network and controls it at runtime.
---

# Topology & Control

<DownloadLLMsFullDoc />

This page covers how the network is generated and wired (`src/topology.star`), how individual nodes
are launched, and the runtime control plane (`src/control_service.star`).

## Network generation — `src/topology.star`

`generate_network_config(plan, rippled_count, goxrpl_count)` produces the shared configuration that
makes every rippled and go-xrpl node part of **one private UNL**:

- Per-node config artifacts: `rippled-{i}.cfg` for each rippled node, `goxrpl-{i}.toml` for each
  go-xrpl node.
- Validator lists in both formats: `validators.txt` (rippled) and `validators.toml` (go-xrpl), each
  carrying every validator's public key.
- Pre-generated secp256k1 validator keypairs (a fixed pool, so runs are reproducible).

Network constants:

| Constant | Value |
| --- | --- |
| `NETWORK_ID` | `10000` (private testnet) |
| Peer port | `51235` |
| RPC port | `5005` |
| WebSocket port | `6006` |
| Quorum | `ceil(0.8 × total_validators)` — matches rippled's 80% rule |

All nodes — rippled and go-xrpl alike — share a single UNL, and quorum is computed across the whole
validator set, so go-xrpl validators are **first-class consensus participants**, not passive
observers.

## Node launchers

### rippled — `src/rippled/rippled.star`
`launch(plan, count, image, network_config)` brings up `count` rippled validators from `image`
(default `rippleci/rippled:2.6.2`). Each node reads `/etc/rippled/rippled-{i}.cfg` and exposes peer
(`51235`), RPC (`5005`, HTTP) and WS (`6006`) ports. Nodes carry the `fuzzer.role: node` label used by
crash detection.

### go-xrpl — `src/goxrpl/goxrpl.star`
`launch(plan, count, image, network_config, name_prefix="goxrpl", enable_chaos_tools=False)` brings up
`count` go-xrpl validators from `image` (default `goxrpl:latest`), running
`server --conf /etc/goxrpl/goxrpl-{i}.toml`. It exposes the same port set and label as rippled.

When `enable_chaos_tools=True` (set automatically for the `chaos` suite), it uses `goxrpl-tools:latest`
— a thin Debian wrapper around the go-xrpl binary that adds `iproute2` and `iptables` so `netem`
latency and partition actions work inside the container. Build it with
`scripts/build-goxrpl-tools.sh` (requires `goxrpl:latest` to exist first).

Each launcher returns node descriptors with `name`, `type`, `service`, `rpc_url`, `ws_url`,
`peer_port` — the shape the oracle, dashboard and control service all consume.

## RPC helpers — `src/helpers/rpc.star`

Shared recipe builders and wait patterns used by every suite:

- `server_info_recipe()` — `server_state`, `peers`.
- `ledger_recipe(ledger_index)` — `ledger_hash`, `account_hash`, `transaction_hash`, `close_time`,
  `ledger_index`.
- `account_info_recipe(account)` — `balance`, `sequence`, `status`.
- `submit_payment_recipe(...)` — sign + submit a payment.
- `wait_for_ledger_seq(plan, node, min_seq, timeout)` — wait for a closed ledger ≥ `min_seq`.
- `wait_for_peers(plan, node, min_peers, timeout)` — wait for peer convergence.
- `assert_validated_ledgers_match(plan, nodes)` — point-in-time hash equality across nodes.

Waits key off `closed_ledger.seq` rather than `validated_ledger`, because a node may not report a
`validated_ledger` in `server_info` in every mode.

## Control plane — `src/control_service.star`

`control_service.launch(plan, rippled_nodes, goxrpl_nodes, scenarios_artifact)` starts the
`confluence-control` service on port `8090`. It is the runtime API behind the `confluence` CLI:

```
/confluence-control \
  --listen :8090 \
  --nodes-config /app/config/nodes.json \
  --scenarios-dir /app/scenarios \
  --poll-interval 5s
```

- `/app/config/nodes.json` — the node list (name, type, RPC URL) it polls.
- `/app/scenarios` — the uploaded `scenarios/` directory.
- It mounts the shared `confluence-findings` volume at `/var/confluence/findings`; a disk-watcher tails
  divergences mirrored there by the fuzz sidecar.

The control service runs two standing oracles for **every** run (no scenario opt-in):

- **`consensus_stall`** — fires when a node's `closed_seq − validated_seq` exceeds the gap threshold
  for a sustained window (tunable via `--stall-gap-threshold` / `--stall-sustain`). Catches the
  "network silently frozen" case that hash comparison alone misses, because hash comparison only ticks
  when `validated_seq` advances.
- **`state_divergence`** — fires when two nodes report different ledger hashes at the same sequence
  (see [Sidecar & Oracle](/sidecar-oracle)).

The CLI reaches all of this over HTTP — `confluence findings`, `confluence finding show`,
`confluence events`, `confluence status`. You can bypass the Kurtosis lookup with
`--control-url http://host:port`.

## Next steps

- [Sidecar & Oracle](/sidecar-oracle) — the differential oracle and findings store.
- [Dashboard](/dashboard) — the live view built on top of these node descriptors.
