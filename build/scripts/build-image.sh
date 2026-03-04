#!/usr/bin/env bash
# Build an Atomic OS OCI image.
# Usage: ./build-image.sh <image-type> [version]
#   image-type: base | cloud | agentic
#   version:    optional, defaults to git describe output

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

IMAGE_TYPE="${1:-agentic}"
VERSION="${2:-$(git -C "$REPO_ROOT" describe --tags --always --dirty 2>/dev/null || echo "dev")}"
REGISTRY="${REGISTRY:-ghcr.io}"
IMAGE_OWNER="${IMAGE_OWNER:-tichomir}"

case "$IMAGE_TYPE" in
  base)
    IMAGE="$REGISTRY/$IMAGE_OWNER/atomic-linux:$VERSION"
    CONTAINERFILE="$REPO_ROOT/images/atomic-base/Containerfile"
    ;;
  cloud)
    IMAGE="$REGISTRY/$IMAGE_OWNER/atomic-cloud:$VERSION"
    CONTAINERFILE="$REPO_ROOT/images/atomic-cloud/Containerfile"
    ;;
  agentic)
    IMAGE="$REGISTRY/$IMAGE_OWNER/agentic-os:$VERSION"
    CONTAINERFILE="$REPO_ROOT/images/agentic-os/Containerfile"
    ;;
  *)
    echo "Unknown image type: $IMAGE_TYPE" >&2
    echo "Usage: $0 <base|cloud|agentic> [version]" >&2
    exit 1
    ;;
esac

echo "==> Building $IMAGE from $CONTAINERFILE"
echo "    Version: $VERSION"
echo "    Repo root: $REPO_ROOT"

podman build \
  --file "$CONTAINERFILE" \
  --tag "$IMAGE" \
  --build-arg "BUILD_VERSION=$VERSION" \
  "$REPO_ROOT"

echo "==> Successfully built $IMAGE"
echo ""
echo "To test this image locally:"
echo "  podman run --rm -it $IMAGE bash"
echo ""
echo "To convert to a bootable disk image:"
echo "  sudo podman run --rm --privileged \\"
echo "    -v \$(pwd)/output:/output \\"
echo "    quay.io/centos-bootc/bootc-image-builder:latest \\"
echo "    --type qcow2 --output /output $IMAGE"
