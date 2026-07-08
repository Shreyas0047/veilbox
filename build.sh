#!/usr/bin/env bash
set -euo pipefail

NAME="Veilbox"
VERSION="2.0"
CODENAME="trixie"

ARCH="${ARCH:-amd64}"
MIRROR="${MIRROR:-http://deb.debian.org/debian}"
OUTPUT_DIR="${OUTPUT_DIR:-$(pwd)/output}"
PARALLEL="${PARALLEL:-$(nproc)}"

usage() {
    cat <<EOF
Usage: $0 [command]

Commands:
  clean     Remove build artifacts
  config    Re-run lb config (uses auto/config)
  build     Run lb build (full ISO build)
  all       clean + config + build
  qemu      Boot the built ISO in QEMU (if available)

Environment variables:
  ARCH       Target architecture (default: amd64)
  MIRROR     Debian mirror URL
  OUTPUT_DIR Output directory for the ISO
  PARALLEL   Parallel jobs for lb build
EOF
    exit 0
}

cmd_clean() {
    echo "==> Cleaning build artifacts..."
    sudo lb clean --purge 2>/dev/null || true
    rm -rf "$OUTPUT_DIR" 2>/dev/null || true
}

cmd_config() {
    echo "==> Running lb config..."
    sudo lb config
}

cmd_build() {
    echo "==> Building Veilbox v2 ISO..."
    mkdir -p "$OUTPUT_DIR"
    sudo lb build 2>&1 | tee "$OUTPUT_DIR/build.log"
    local iso
    iso=$(ls -t live-image-*.hybrid.iso 2>/dev/null | head -1)
    if [ -n "$iso" ]; then
        mv "$iso" "$OUTPUT_DIR/veilbox-${VERSION}-${ARCH}.iso"
        echo "==> ISO built: $OUTPUT_DIR/veilbox-${VERSION}-${ARCH}.iso"
    fi
}

cmd_qemu() {
    local iso
    iso=$(ls -t "$OUTPUT_DIR"/veilbox-*.iso 2>/dev/null | head -1)
    if [ -z "$iso" ]; then
        echo "No ISO found in $OUTPUT_DIR" >&2
        exit 1
    fi
    echo "==> Booting $iso in QEMU..."
    qemu-system-x86_64 -m 4096 -smp 4 \
        -enable-kvm \
        -cdrom "$iso" \
        -netdev user,id=net0 \
        -device virtio-net,netdev=net0 \
        -display gtk
}

case "${1:-help}" in
    clean) cmd_clean ;;
    config) cmd_config ;;
    build) cmd_build ;;
    all) cmd_clean; cmd_config; cmd_build ;;
    qemu) cmd_qemu ;;
    *) usage ;;
esac
