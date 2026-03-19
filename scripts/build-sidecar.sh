#!/usr/bin/env bash
# Build the trafficgen sidecar Docker image.
#
# Usage:
#   ./scripts/build-sidecar.sh [IMAGE_TAG]
#
# Default tag: trafficgen:latest

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SIDECAR_DIR="$SCRIPT_DIR/../sidecar"
IMAGE_TAG="${1:-trafficgen:latest}"

echo "Building trafficgen sidecar image: $IMAGE_TAG"
docker build -t "$IMAGE_TAG" "$SIDECAR_DIR"
echo "Done: $IMAGE_TAG"
