#!/usr/bin/env bash
# Push Atomic OS OCI images to a container registry.
# Usage: ./push-image.sh [version]

set -euo pipefail

VERSION="${1:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}"
REGISTRY="${REGISTRY:-ghcr.io}"
IMAGE_OWNER="${IMAGE_OWNER:-tichomir}"

IMAGES=(
  "$REGISTRY/$IMAGE_OWNER/atomic-linux:$VERSION"
  "$REGISTRY/$IMAGE_OWNER/atomic-cloud:$VERSION"
  "$REGISTRY/$IMAGE_OWNER/agentic-os:$VERSION"
)

echo "==> Pushing Atomic OS images (version: $VERSION)"

for image in "${IMAGES[@]}"; do
  echo "  Pushing $image..."
  podman push "$image"
done

echo "==> All images pushed successfully"
echo ""
echo "Images available at:"
for image in "${IMAGES[@]}"; do
  echo "  $image"
done
