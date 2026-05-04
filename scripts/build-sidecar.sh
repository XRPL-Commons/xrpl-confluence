#!/usr/bin/env bash
# Build the xrpl-confluence sidecar Docker image.
#
# Usage:
#   ./scripts/build-sidecar.sh [IMAGE_TAG]
#
# Default tag: xrpl-confluence-sidecar:latest

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SIDECAR_DIR="$SCRIPT_DIR/../sidecar"
IMAGE_TAG="${1:-xrpl-confluence-sidecar:latest}"

echo "Building sidecar image: $IMAGE_TAG"
docker build -t "$IMAGE_TAG" "$SIDECAR_DIR"
echo "Done: $IMAGE_TAG"
