#!/usr/bin/env bash
# Bisect a fuzz run log to its minimal failing prefix.
#
# Usage:
#   scripts/shrink.sh <run-log.ndjson> <divergence.json> [seed]
#
# Env:
#   ENCLAVE_NAME   default: shrink-probe
#   PKG_PATH       default: $(pwd)              (path to the Kurtosis package = repo root)
#   ACCOUNTS       default: 10                  (must match the original run)
#   GOXRPL_IMAGE   default: goxrpl:latest
#   RIPPLED_IMAGE  default: rippleci/rippled:2.6.2
#
# Each probe spins up a fresh enclave and runs the `shrink` test suite with a
# specific shrink_max_step; reads /status from the sidecar to learn whether
# the original divergence reproduced; binary-searches K in [0, N-1] to find
# the smallest matching prefix.

set -euo pipefail

RUN_LOG=${1:?"run log path required"}
DIV=${2:?"divergence JSON path required"}
SEED=${3:-}

ENCLAVE_NAME=${ENCLAVE_NAME:-shrink-probe}
PKG_PATH=${PKG_PATH:-$(pwd)}
ACCOUNTS=${ACCOUNTS:-10}
GOXRPL_IMAGE=${GOXRPL_IMAGE:-goxrpl:latest}
RIPPLED_IMAGE=${RIPPLED_IMAGE:-rippleci/rippled:2.6.2}

if ! command -v kurtosis >/dev/null 2>&1; then
    echo "error: kurtosis CLI not found in PATH" >&2
    exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
    echo "error: jq not found in PATH" >&2
    exit 1
fi

N=$(grep -c '' "$RUN_LOG")
if [[ "$N" -lt 1 ]]; then
    echo "error: empty run log: $RUN_LOG" >&2
    exit 1
fi
echo "[shrink] run log has $N entries; bisecting K in [0, $((N - 1))]"

# Stage inputs for upload as one Kurtosis files artifact.
STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT
cp "$RUN_LOG" "$STAGE/run.ndjson"
cp "$DIV"     "$STAGE/div.json"

build_args() {
    local k=$1
    local seed_field=""
    if [[ -n "$SEED" ]]; then
        seed_field=", \"seed\": $SEED"
    fi
    cat <<EOF
{
  "rippled_image": "$RIPPLED_IMAGE",
  "goxrpl_image":  "$GOXRPL_IMAGE",
  "test_suite":    "shrink",
  "shrink_args": {
    "shrink_artifact":  "shrink-input",
    "shrink_max_step":  $k,
    "accounts":         $ACCOUNTS$seed_field
  }
}
EOF
}

probe() {
    local k=$1
    echo "[shrink] probe k=$k" >&2

    kurtosis enclave rm --force "$ENCLAVE_NAME" >/dev/null 2>&1 || true
    kurtosis enclave add --name "$ENCLAVE_NAME" >/dev/null

    # Upload run.ndjson + div.json as artifact "shrink-input" before kurtosis run.
    kurtosis files upload "$ENCLAVE_NAME" "$STAGE" --name shrink-input >/dev/null

    local args_file
    args_file=$(mktemp)
    build_args "$k" > "$args_file"
    kurtosis run --enclave "$ENCLAVE_NAME" "$PKG_PATH" --args-file "$args_file" >/dev/null
    rm -f "$args_file"

    # Sidecar's /status is exposed on the "fuzz" service, port "results".
    local port_line
    port_line=$(kurtosis port print "$ENCLAVE_NAME" fuzz results)
    local url="${port_line##* }"

    local matched
    matched=$(curl -fsS "$url/status" | jq -r '.shrink.matched // false')
    echo "[shrink]   matched=$matched" >&2
    echo "$matched"
}

# Sanity: the full log MUST reproduce. If not, refuse to bisect (we'd be
# bisecting a flake).
hi=$((N - 1))
echo "[shrink] verifying full log (k=$hi) reproduces..."
if [[ "$(probe $hi)" != "true" ]]; then
    echo "[shrink] full log did not reproduce — aborting" >&2
    exit 2
fi
best=$hi

# Binary search: smallest K with matched=true.
lo=0
hi=$((N - 2))
while [[ $lo -le $hi ]]; do
    mid=$(( (lo + hi) / 2 ))
    if [[ "$(probe $mid)" == "true" ]]; then
        best=$mid
        hi=$((mid - 1))
    else
        lo=$((mid + 1))
    fi
done

OUT="${RUN_LOG%.ndjson}_shrunk_k${best}.ndjson"
head -n $((best + 1)) "$RUN_LOG" > "$OUT"
echo "[shrink] minimal prefix: $((best + 1)) tx(s) (max_step=$best)"
echo "[shrink] wrote $OUT"
