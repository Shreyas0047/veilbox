# SPDX-License-Identifier: GPL-2.0-only
#
# Veilbox Builder — Docker image for building Veilbox OS from source.
#
# All dependencies are pre-installed.  Run the build with:
#
#   docker build -t veilbox-builder .
#   docker run --rm -v "$(pwd):/build" -w /build veilbox-builder ./build.sh
#
# To extract build artifacts directly (no volume mount required):
#
#   docker build --output=output/ .
#

FROM fedora:latest AS veilbox-builder

RUN dnf install -y \
    bc \
    bison \
    bzip2 \
    cryptsetup \
    dwarves \
    e2fsprogs \
    elfutils-libelf-devel \
    flex \
    gcc \
    git \
    git-lfs \
    golang \
    grub2-pc-modules \
    grub2-tools \
    iptables-legacy \
    make \
    ncurses-devel \
    nftables \
    openssl-devel \
    qemu-img \
    qemu-system-x86 \
    rsync \
    squashfs-tools \
    sudo \
    tar \
    wget \
    xz \
    && dnf clean all

WORKDIR /build

CMD ["./build.sh"]

# Multi-stage artifact extraction target.
# Usage: docker build --output=output/ .
FROM scratch AS artifact
COPY --from=veilbox-builder /build/output/ /
