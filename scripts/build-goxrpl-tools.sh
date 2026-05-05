#!/usr/bin/env bash
# Build goxrpl-tools:latest by combining goxrpl:latest with iproute2/iptables.
# Used by the chaos suite to enable LatencyEvent / PartitionEvent against
# goXRPL containers (the vanilla distroless goxrpl image lacks these tools).
set -euo pipefail

cd "$(dirname "$0")/.."

if ! docker image inspect goxrpl:latest --format '{{.Id}}' >/dev/null 2>&1; then
	echo "goxrpl:latest not present — build it from the goXRPL repo first" >&2
	exit 1
fi

docker build -t goxrpl-tools:latest -f goxrpl-tools.Dockerfile .
echo "built goxrpl-tools:latest"
