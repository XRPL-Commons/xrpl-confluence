---
description: Build and run a persistent local rippled network with participant-supplied images.
---

# Custom local networks

This guide runs a persistent, rippled-only network from a binary that you build or already have. It
is useful for protocol experiments, amendment work, and testing a change across several validator
instances. The default participant tag in the examples is `rippled-hackathon:local`; it is a local
convention, not an upstream image that Confluence downloads.

Local networks use `workload.kind: none`. They stay up until you run `confluence down` or explicitly
reset them. `confluence run` is a bounded workload command and rejects `none`; use `confluence up`
for this workflow.

## What you need

Install and start the [Kurtosis CLI](https://docs.kurtosis.com/install/) engine, Docker, and Go
1.25 or newer. Keep `curl` and `jq` available for the RPC example below. You also need either:

- a Linux `rippled` executable that you built for the target Docker architecture; or
- an existing Docker image containing a compatible `rippled` executable.

Confluence does not compile rippled and cannot make arbitrary binaries compatible. The supplied
binary and the image runtime must have the same Linux architecture (for example, `linux/amd64`),
and a dynamically linked binary must run against the Ubuntu 22.04 glibc and C++ ABI. Its additional
shared libraries must be present in the image. A binary built on a newer distribution may require a
newer runtime, while a binary built for another architecture will not start. Check the exact
rippled version and build flags when comparing nodes.

The multi-validator mode supports 2 through 10 total validators. A one-node standalone network is
outside this mode. The examples use three rippled validators and `goxrpl.count: 0`: custom genesis
and network settings currently require a rippled-only topology so a mixed genesis cannot silently
diverge.

## Build or check a participant image

From the repository root, build the CLI first:

```bash
( cd sidecar && go build -o ../bin/confluence ./cmd/confluence )
export PATH="$PWD/bin:$PATH"
confluence version
```

The participant Dockerfile is [`images/rippled/Dockerfile`](https://github.com/XRPL-Commons/xrpl-confluence/blob/main/images/rippled/Dockerfile).
It expects a precompiled binary, installs the Ubuntu 22.04 runtime libraries commonly used by
rippled, places the executable at `/usr/local/bin/rippled`, and uses that path as an exec-form
entrypoint. The database directory `/var/lib/rippled/db` is created writable; Confluence mounts
persistent enclave storage there.

The helper accepts the binary path and an optional image tag. It stages the binary in a temporary
Docker context, so the binary may live outside this repository:

```bash
./scripts/build-rippled.sh /absolute/path/to/rippled rippled-hackathon:local
./scripts/check-rippled-image.sh rippled-hackathon:local
```

When the host architecture differs from the binary, select the target platform explicitly. For an
amd64 binary built on an arm64 host, for example:

```bash
DOCKER_DEFAULT_PLATFORM=linux/amd64 \
  ./scripts/build-rippled.sh /absolute/path/to/rippled rippled-hackathon:local
```

The image build rejects unresolved shared libraries when `ldd` reports them. The preflight starts
short-lived containers to run `--version` and create a probe file under
`/var/lib/rippled/db`; it does not start a node or modify a host directory. For an existing image
whose binary is at another path, pass that path as the second argument:

```bash
./scripts/check-rippled-image.sh registry.example/rippled:3.4.0 /opt/rippled/bin/rippled
```

The image must provide `/bin/sh` for the storage probe. A custom `topology.rippled.entrypoint`
must name an executable in the selected image and must accept the command arguments Confluence
adds (`--conf <file> --start` for fresh networks and `--conf <file> --load` on resume). Keep image tags explicit and immutable for a reproducible run;
Confluence makes no promise about an arbitrary or moving `latest` tag.

If the control service is enabled (the default), build the local sidecar image too. The dashboard
uses its own Node image and does not require this build:

```bash
./scripts/build-sidecar.sh xrpl-confluence-sidecar:latest
```

If you use a different sidecar tag, set `services.sidecar_image` to the same tag in the scenario.

## Start the simple network

Review [`scenarios/local-network.yaml`](https://github.com/XRPL-Commons/xrpl-confluence/blob/main/scenarios/local-network.yaml), then validate and start it:

```bash
confluence scenario validate scenarios/local-network.yaml
confluence up -f scenarios/local-network.yaml --wait-network 3m
```

`up` waits for every node to advance validated ledgers and agree on a common ledger index and hash.
The default readiness deadline is 180 seconds; `--wait-network` changes it. A connected RPC port
alone does not mean that the network is ready. If readiness fails, inspect each node's logs before
changing the timeout.

The endpoint mapping is refreshed from Kurtosis on demand:

```bash
confluence endpoints
confluence endpoints --json | jq
```

The JSON form has `nodes`, with `name`, `rpc`, `ws`, and `peer` URLs for each node, plus optional
`dashboard_url` and `control_url` fields. Copy the `rpc` URL for `rippled-0` into `RPC_URL` when
following the payment example:

```bash
RPC_URL="$(confluence endpoints --json | jq -r '.nodes[] | select(.name == "rippled-0") | .rpc')"
test -n "$RPC_URL" && test "$RPC_URL" != "null"
confluence status -w
confluence logs -n rippled-0 -f
```

The dashboard is enabled in the simple example. Its URL is reported by `endpoints`; the control
URL is useful for the CLI's findings and event commands. You can set both `services.dashboard` and
`services.control` to `false` for a node-only network. A workload runner requires control, but
`none` permits both optional services to be disabled. With control disabled, node RPC,
`endpoints`, and `logs` remain available; `status`, findings, and events require control. Changing
service flags on an existing network requires `--reset`.

## Submit a disposable funded payment

The genesis account in a fresh standalone rippled ledger is
`rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh`, derived from the hard-coded test-only seed
`snoPBrXtMeMyMHUVTgbuqAfg1SUTb` (derived from `masterpassphrase`). Treat both as public disposable test credentials. Never use them on a public
network or with funds that matter.

The following uses rippled's existing JSON-RPC `wallet_propose`, `account_info`, `sign`, and
`submit` methods. It creates a destination, signs a 20 XRP payment offline with the genesis secret,
funds above the default 10 XRP account reserve, and includes `NetworkID: 10000` in the transaction. Replace this flow with a client library and a
real private key for anything beyond a disposable local network.

```bash
GENESIS="rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"

WALLET_JSON="$(curl -sS "$RPC_URL" \
  -H 'content-type: application/json' \
  --data '{"method":"wallet_propose","params":[{}]}')"
DESTINATION="$(jq -r '.result.account_id // .result.account // empty' <<<"$WALLET_JSON")"
test -n "$DESTINATION" && test "$DESTINATION" != "null"

SEQUENCE="$(curl -sS "$RPC_URL" \
  -H 'content-type: application/json' \
  --data "$(jq -nc --arg account "$GENESIS" \
    '{method:"account_info",params:[{account:$account,ledger_index:"validated"}]}')" \
  | jq -r '.result.account_data.Sequence')"

TX_JSON="$(jq -nc --arg account "$GENESIS" --arg destination "$DESTINATION" \
  --argjson sequence "$SEQUENCE" '{
    TransactionType: "Payment",
    Account: $account,
    Destination: $destination,
    Amount: "20000000",
    Fee: "12",
    Sequence: $sequence,
    NetworkID: 10000
  }')"

TX_BLOB="$(curl -sS "$RPC_URL" \
  -H 'content-type: application/json' \
  --data "$(jq -nc --arg secret snoPBrXtMeMyMHUVTgbuqAfg1SUTb --argjson tx "$TX_JSON" '{
    method: "sign",
    params: [{offline: true, secret: $secret, tx_json: $tx}]
  }')" \
  | jq -r '.result.tx_blob')"
test -n "$TX_BLOB" && test "$TX_BLOB" != "null"

curl -sS "$RPC_URL" \
  -H 'content-type: application/json' \
  --data "$(jq -nc --arg blob "$TX_BLOB" \
    '{method:"submit",params:[{tx_blob:$blob}]}')" | jq
```

Wait for the next validated ledger, then query the submitted hash from the `submit` result with
`tx` or watch `confluence status -w`. A result such as `tesSUCCESS` means the transaction was
accepted; confirmation in a validated ledger is the useful end-to-end check.

## Use per-node images and configuration

[`scenarios/local-network-mixed.yaml`](https://github.com/XRPL-Commons/xrpl-confluence/blob/main/scenarios/local-network-mixed.yaml)
keeps three rippled validators but gives `rippled-1` a separately built image tag and a different
`transaction_queue` section. Build and preflight both tags before launch:

```bash
./scripts/build-rippled.sh /absolute/path/to/rippled rippled-hackathon:local
./scripts/build-rippled.sh /absolute/path/to/rippled-canary rippled-hackathon:local-canary
./scripts/check-rippled-image.sh rippled-hackathon:local-canary
confluence scenario validate scenarios/local-network-mixed.yaml
confluence up -f scenarios/local-network-mixed.yaml --wait-network 3m
```

`topology.rippled.nodes` must contain exactly `count` entries. Node names remain
`rippled-0` through `rippled-<count-1>`, regardless of which image each entry selects. A node
override can set `image`, `entrypoint`, and `config`; an explicitly supplied image must be
non-empty. Group settings provide defaults, and a per-node config entry replaces that complete INI
section for the node. Config values are individual single-line entries: section headers, embedded
newlines, and section injection are rejected.

Confluence owns the network-bearing sections. Do not put these names in `topology.rippled.config`:
`server`, `port_peer`, `port_rpc`, `port_ws`, `node_db`, `database_path`, `debug_logfile`,
`ips_fixed`, `validation_seed`, `validation_quorum`, `validators_file`, `network_id`,
`amendments`, and `veto_amendments`. The compiler generates those values from the topology and
`network` fields so every participant gets the same private network. Use remaining sections, such
as `transaction_queue` or `node_size`, for a full section override.

## Network IDs and amendments

`network.network_id` defaults to `10000`; set it explicitly when documenting or reproducing a
network. `amendments` and `veto_amendments` each contain `{id, name}` objects with a 64-character
hexadecimal amendment ID. An amendment listed in both sets is invalid.

These fields are rippled amendment **votes at genesis**. An upvote does not force a feature that the
binary does not implement, and a default set of amendments supported by the image may still apply.
Use `veto_amendments` to disable a feature for an experiment. An explicitly supplied empty
`amendments: []` replaces the default upvote list. Keep amendment IDs and names aligned with the
exact rippled build under test, and reset the network after changing them; a resume deliberately
keeps the existing genesis state.

Run these commands from the same working directory: Confluence saves enclave metadata in
`.confluence/networks/` there. The network and service settings are part of resume identity. `--resume` may change participant
images, entrypoints, and per-node config, but it must keep the validator counts, network/genesis
settings, and enabled service flags stable. Validator keys are fixed public test fixtures. Persistent
storage is enclave-scoped; a different enclave name has different data.

## Rebuild and resume

`--resume` is the supported restart workflow. It discovers the existing enclave and its saved
scenario metadata, keeps the persistent `rippled-N-data` directories, and can update services to a
rebuilt image with the same tag. For example, rebuild the binary and resume the simple network:

```bash
./scripts/build-rippled.sh /absolute/path/to/new-rippled rippled-hackathon:local
./scripts/check-rippled-image.sh rippled-hackathon:local
confluence up --resume -f scenarios/local-network.yaml --wait-network 3m
```

If you maintain your own Dockerfile and build context, `up` can rebuild that context using the
scenario's group image tag before resuming:

```bash
confluence up --resume -f scenarios/local-network.yaml \
  --rebuild-rippled /absolute/path/to/docker-build-context \
  --wait-network 3m
```

Changing a genesis-affecting setting (validator count, `network_id`, amendment votes, or vetoes)
requires a fresh network. Ask `up` to remove the existing enclave and its data explicitly:

```bash
confluence up --reset -f scenarios/local-network.yaml --wait-network 3m
```

To finish a session and remove its persistent data:

```bash
confluence down
```

A raw container restart of a node that was launched with `--start` is not a resume operation. It
does not perform Confluence's enclave discovery, topology checks, image update, or readiness
comparison; use `confluence up --resume` instead.

## Next steps

- [Quickstart](/quickstart) — the regular mixed-network workflow.
- [CLI & Scenarios](/cli) — command and schema reference.
- [Topology & Control](/topology) — generated node configuration and control endpoints.
