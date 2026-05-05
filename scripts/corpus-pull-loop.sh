#!/usr/bin/env bash
# Periodically extract /output/corpus from a Kurtosis-managed sidecar to a
# host path. Designed for week-long soak runs where leaving "make soak-pull"
# in a manual loop is brittle.
#
# Usage: corpus-pull-loop.sh <enclave> <service> <interval-seconds> <host-dir>
set -euo pipefail

if [[ $# -ne 4 ]]; then
	echo "usage: $0 <enclave> <service> <interval-seconds> <host-dir>" >&2
	exit 2
fi

ENCLAVE="$1"
SERVICE="$2"
INTERVAL="$3"
HOSTDIR="$4"

mkdir -p "$HOSTDIR"

while true; do
	UUID=$(kurtosis service inspect "$ENCLAVE" "$SERVICE" 2>/dev/null \
		| awk '/^UUID:/ {print $2; exit}' || true)
	if [[ -n "$UUID" ]]; then
		CONTAINER=$(docker ps --format '{{.Names}}' | grep "^$SERVICE--$UUID" | head -1 || true)
		if [[ -n "$CONTAINER" ]]; then
			docker cp "$CONTAINER:/output/corpus" "$HOSTDIR/" 2>/dev/null \
				&& echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] pulled $SERVICE corpus to $HOSTDIR/" \
				|| echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] $SERVICE corpus pull skipped (transient)" >&2
		fi
	fi
	sleep "$INTERVAL"
done
