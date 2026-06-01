---
description: The live dashboard for watching every node's ledger, peers and logs in real time.
---

# Dashboard

<DownloadLLMsFullDoc />

Every Confluence run launches a **live dashboard** that gives a single-pane view of the whole network:
each node's ledger progression, peer topology, logs, and (when present) fuzz metrics. It is launched by
`src/dashboard/dashboard.star` from the source in `dashboard/`.

## Running it

The dashboard starts automatically with the network. Its URL is reported on startup — for example,
`make soak` prints:

```
Dashboard: http://<ip>:8080
```

It runs on the `node:22-alpine` image on port `8080` (Node 22 supports the WebSocket client globally,
without experimental flags).

## How it works

The server (`dashboard/server.js`) keeps a live model of the network:

- It opens an XRPL **WebSocket `subscribe` stream** to each node for real-time ledger-close events.
- It **polls each node's RPC** every `poll_interval_ms` (default `5000`) for coarser state — peers and
  `server_state`.
- It keeps a rolling per-node log buffer.
- It pushes updates to the browser over **Server-Sent Events (SSE)**.

Its configuration (`config.json`) carries the node list (each with `name`, `type`, an HTTP `rpc` URL
and a WebSocket `ws` URL), the `poll_interval_ms`, and an optional `fuzz_metrics_url`.

HTTP endpoints:

| Endpoint | Returns |
| --- | --- |
| `/` | The dashboard UI |
| `/api/nodes` | Current per-node state |
| `/api/logs` | Recent per-node logs |
| `/api/metrics` | Fuzz metrics (when a `fuzz_metrics_url` is configured) |

## The UI

The front-end (`dashboard/app.js`, vanilla JS, no build step) offers several views, switched by
hash-based routing:

- **Timeline** — ledger closes across all nodes over time, with divergences highlighted.
- **Chains** — each node's chain of recent ledgers side by side.
- **Topology** — the peer graph.
- **Fuzzer** — fuzz metrics, when available.
- **Block** — a selected ledger in detail.

Filters let you narrow by node-name regex, timeline regex, divergences-only, and log level
(ok / warn / err), with a follow-tail toggle. Mock fixtures (`mock*.json`) are bundled so the UI can be
opened offline for development.

## Grafana (optional)

For long soak runs, enable observability (`OBSERVABILITY=1`, a scenario's `observability.enabled`, or
the CLI's `--with-dashboard`) to bring up Prometheus + Grafana alongside the dashboard. Grafana runs on
port `3000` as an anonymous viewer with the Prometheus datasource pre-provisioned from
`dashboard/grafana-provisioning/`. See [Sidecar & Oracle](/sidecar-oracle#optional-observability).

## Next steps

- [Sidecar & Oracle](/sidecar-oracle) — what feeds the divergence highlights.
- [CLI & Scenarios](/cli) — `confluence status` for a terminal-side view.
