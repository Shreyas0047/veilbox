#!/usr/bin/env bash
set -euo pipefail

NAME="veilbox-builder"
IMAGE="veilbox-builder:latest"
REPO_DIR="$(cd "$(dirname "$0")" && pwd)"
OUTPUT_DIR="${OUTPUT_DIR:-$REPO_DIR/output}"
BUILD_CMD="${BUILD_CMD:-build}"

# Detect container runtime
RUNTIME=""
if command -v podman &>/dev/null; then
    RUNTIME="podman"
elif command -v docker &>/dev/null; then
    RUNTIME="docker"
else
    echo "ERROR: Neither podman nor docker found" >&2
    exit 1
fi

echo "==> Using $RUNTIME"

# Build the builder image
echo "==> Building builder image..."
$RUNTIME build -t "$IMAGE" -f Dockerfile.build .

# Ensure output directory exists
mkdir -p "$OUTPUT_DIR"

# Run the build
echo "==> Running lb build inside Debian container..."
$RUNTIME run --rm \
    --name "$NAME" \
    --privileged \
    -v "$REPO_DIR:/repo:Z" \
    -v "$OUTPUT_DIR:/repo/output:Z" \
    "$IMAGE" \
    -c "cd /repo && bash ./build.sh $BUILD_CMD"

echo "==> Build complete. ISO in: $OUTPUT_DIR"
