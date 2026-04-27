# Fuzz Shrinker (M3c)

Given a fuzz run log that triggered a divergence, the shrinker finds the
smallest prefix of the log that still reproduces *the same* divergence.

## Architecture

The sidecar's shrinker is a **single probe** — it replays a prefix of length
`SHRINK_MAX_STEP+1` against a fresh enclave and reports whether the original
divergence's signature was reproduced. Bisection lives outside the sidecar in
`scripts/shrink.sh`, which orchestrates one Kurtosis enclave per probe.

This split is forced by the Kurtosis lifecycle: resetting the topology means
tearing down the enclave that hosts the sidecar.

```
┌──────────────────────────────────────────────────┐
│  scripts/shrink.sh   (bisect loop on host)       │
│    ├─ kurtosis enclave rm --force                │
│    ├─ kurtosis files upload (run.ndjson, div)    │
│    ├─ kurtosis run --args {shrink_max_step: K}   │
│    └─ curl <sidecar>/status → matched? y/n       │
└──────────────────────────────────────────────────┘
                    │ per-probe ↓
┌──────────────────────────────────────────────────┐
│  Kurtosis enclave (one fresh per probe)          │
│    rippled × N + goXRPL × M + fuzz sidecar       │
│      (MODE=shrink)                               │
│        1. fund + setup state                     │
│        2. replay log[step ≤ MaxStep] with        │
│           per-tx WaitTxValidated + layer 2/3     │
│        3. compare observed divergences           │
│           against signature loaded from div.json │
│        4. write <corpus>/shrinks/..._result.json │
│        5. expose /status                         │
└──────────────────────────────────────────────────┘
```

## Inputs

- A run log: `<corpus>/runs/<seed>.ndjson` (produced by any fuzz/replay run
  since M3b).
- A divergence JSON: `<corpus>/divergences/<timestamp>_<n>.json` (produced by
  the same run; the shrinker derives a `DivergenceSignature` from `kind` plus
  one of `tx_type` / `field` / `invariant`).

## Run

```
scripts/shrink.sh \
  /path/to/<seed>.ndjson \
  /path/to/<timestamp>_<n>.json \
  [seed]
```

The third argument (seed) is optional but **strongly recommended**: passing
the same seed used by the original run keeps the deterministic account pool
stable, so the log's `Account` / `Destination` fields still match real,
funded accounts.

The script prints the minimal prefix length and writes
`<seed>_shrunk_k<K>.ndjson` next to the input log.

### Environment overrides

| Var             | Default                  | Meaning                                                |
|-----------------|--------------------------|--------------------------------------------------------|
| `ENCLAVE_NAME`  | `shrink-probe`           | Kurtosis enclave name (recreated per probe).           |
| `PKG_PATH`      | `$(pwd)`                 | Path to the Kurtosis package (this repo).              |
| `ACCOUNTS`      | `10`                     | Account pool size — must match the original run.       |
| `GOXRPL_IMAGE`  | `goxrpl:latest`          | goXRPL image tag.                                      |
| `RIPPLED_IMAGE` | `rippleci/rippled:2.6.2` | rippled image tag.                                     |

## Signature matching

The shrinker considers a probe "matched" iff at least one observed divergence
has the same signature as the original. The signature is the tuple:

| Original `kind` | Required match on                         |
|-----------------|-------------------------------------------|
| `tx_result`     | `kind` + `details.tx_type`                |
| `metadata`      | `kind` + `details.tx_type`                |
| `state_hash`    | `kind` + `details.comparison.divergences[0].field` |
| `invariant`     | `kind` + `details.invariant`              |

A bare `kind` match would let the bisect collapse onto a *different* bug;
the kind-specific subfield is required.

## Caveats

- Each probe spins up a fresh enclave, runs `FundFromGenesis` + `SetupState`,
  then replays the prefix. Expect a few minutes per probe.
- Layer-1 (state-hash) divergences require a full ledger close. The shrinker
  currently checks layer-2 (tx_result) and layer-3 (metadata) per tx.
  Layer-1 shrinking lands in M5 once the controlled-close runner is available.
- The driver aborts if the **full** log does not reproduce on the very first
  probe — bisecting a flaky reproducer would yield meaningless results.
- Setup is deterministic given the seed; pass it through to the driver.
