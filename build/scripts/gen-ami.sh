#!/usr/bin/env bash
# Convert the agentic-os OCI image to an AWS AMI using bootc-image-builder.
# Requires: AWS credentials in environment, podman, bootc-image-builder
# Usage: ./gen-ami.sh [version]

set -euo pipefail

VERSION="${1:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}"
REGISTRY="${REGISTRY:-ghcr.io}"
IMAGE_OWNER="${IMAGE_OWNER:-tichomir}"
OUTPUT_DIR="${OUTPUT_DIR:-$(pwd)/output/ami}"

AGENTIC_OS_IMAGE="$REGISTRY/$IMAGE_OWNER/agentic-os:$VERSION"

# Verify AWS credentials are present
if [[ -z "${AWS_ACCESS_KEY_ID:-}" ]] || [[ -z "${AWS_SECRET_ACCESS_KEY:-}" ]]; then
  echo "Error: AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY must be set" >&2
  exit 1
fi

AWS_REGION="${AWS_DEFAULT_REGION:-us-east-1}"

echo "==> Building AWS AMI from $AGENTIC_OS_IMAGE"
echo "    Region: $AWS_REGION"
echo "    Output: $OUTPUT_DIR"

mkdir -p "$OUTPUT_DIR"

sudo podman run \
  --rm \
  --privileged \
  --pull=newer \
  -v "$OUTPUT_DIR:/output" \
  -e AWS_ACCESS_KEY_ID \
  -e AWS_SECRET_ACCESS_KEY \
  -e AWS_DEFAULT_REGION="$AWS_REGION" \
  quay.io/centos-bootc/bootc-image-builder:latest \
  --type ami \
  --output /output \
  --aws-ami-name "agentic-os-$VERSION" \
  --aws-region "$AWS_REGION" \
  "$AGENTIC_OS_IMAGE"

echo "==> AMI build complete"
echo "    Check AWS console for AMI: agentic-os-$VERSION"
