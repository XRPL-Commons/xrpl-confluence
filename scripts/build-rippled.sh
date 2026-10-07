#!/usr/bin/env bash
# Build the participant runtime image around a precompiled rippled binary.
#
# Usage:
#   ./scripts/build-rippled.sh /path/to/rippled [IMAGE_TAG]
#
# The binary is copied into a temporary Docker build context, so it may live
# outside this repository. Docker builds for the host architecture by default;
# set DOCKER_DEFAULT_PLATFORM (for example, linux/amd64 on an arm64 host) when
# the binary and runtime architecture differ. The default tag is
# rippled-hackathon:local.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

if [[ $# -lt 1 || $# -gt 2 ]]; then
	printf 'usage: %s /path/to/rippled [IMAGE_TAG]\n' "$0" >&2
	exit 2
fi

BINARY_INPUT="$1"
IMAGE_TAG="${2:-rippled-hackathon:local}"

if [[ "$BINARY_INPUT" = /* ]]; then
	BINARY_PATH="$BINARY_INPUT"
else
	BINARY_PATH="$PWD/$BINARY_INPUT"
fi

if [[ ! -f "$BINARY_PATH" || ! -x "$BINARY_PATH" ]]; then
	printf 'rippled binary must be an executable file: %s\n' "$BINARY_PATH" >&2
	exit 1
fi

BUILD_CONTEXT="$(mktemp -d "${TMPDIR:-/tmp}/confluence-rippled-build.XXXXXX")"
trap 'rm -rf "$BUILD_CONTEXT"' EXIT

cp "$REPO_ROOT/images/rippled/Dockerfile" "$BUILD_CONTEXT/Dockerfile"
cp "$BINARY_PATH" "$BUILD_CONTEXT/rippled"

printf 'Building %s from %s\n' "$IMAGE_TAG" "$BINARY_PATH"
DOCKER_BUILD_ARGS=()
if [[ -n "${DOCKER_DEFAULT_PLATFORM:-}" ]]; then
	DOCKER_BUILD_ARGS+=(--platform "$DOCKER_DEFAULT_PLATFORM")
fi
docker build "${DOCKER_BUILD_ARGS[@]}" -t "$IMAGE_TAG" "$BUILD_CONTEXT"
printf 'Built %s\n' "$IMAGE_TAG"
