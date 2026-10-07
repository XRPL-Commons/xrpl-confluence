#!/usr/bin/env bash
# Validate a rippled participant image before starting a Kurtosis enclave.
#
# Usage:
#   ./scripts/check-rippled-image.sh IMAGE [ENTRYPOINT]
#
# The optional ENTRYPOINT is useful for an existing compatible image whose
# binary is not /usr/local/bin/rippled. The image must also provide /bin/sh for
# the writable-storage probe. The probe uses an ephemeral container and does
# not touch a host directory or a running enclave.

set -euo pipefail

if [[ $# -lt 1 || $# -gt 2 ]]; then
	printf 'usage: %s IMAGE [ENTRYPOINT]\n' "$0" >&2
	exit 2
fi

IMAGE="$1"
ENTRYPOINT="${2:-/usr/local/bin/rippled}"

if ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
	printf 'Docker image is not available locally: %s\n' "$IMAGE" >&2
	exit 1
fi

printf 'Checking %s --version via %s\n' "$IMAGE" "$ENTRYPOINT"
docker run --rm --entrypoint "$ENTRYPOINT" "$IMAGE" --version

printf 'Checking writable /var/lib/rippled/db in %s\n' "$IMAGE"
docker run --rm --entrypoint /bin/sh "$IMAGE" -eu -c '
db=/var/lib/rippled/db
mkdir -p "$db"
test -w "$db"
probe="$db/.confluence-preflight-$$"
: >"$probe"
rm -f "$probe"
'

printf 'Image preflight passed: %s\n' "$IMAGE"
