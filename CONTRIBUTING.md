# Contributing to Veilbox

Be respectful, constructive, and assume good faith.

## Getting Started

```bash
git clone https://github.com/Shreyas0047/veilbox.git
cd veilbox
git lfs pull
./build.sh           # Configure kernel, build rootfs, create disk image
MEM=2G ./test.sh     # Boot in QEMU
```

## Development Workflow

1. Create a feature branch from `main`
2. Make changes to kernel config, rootfs, or build scripts
3. Rebuild and test: `./build.sh && ./test.sh --check`
4. Commit with a descriptive message and open a PR

## Project Architecture

```
build.sh:  Kernel config → rootfs → disk image → VDI
test.sh:   QEMU/VirtualBox test runner
rootfs/:   Initramfs source (inittab, rcS, configs, binaries)
kernel/:   Kernel config fragment (custom-os.config)
```

## Pull Request Guidelines

- One feature per PR. Small, focused changes are easier to review.
- Rebase on `main` before opening.
- Include health check output: `./test.sh --check`
- Don't force-push after review has started.

## Coding Standards

- Shell scripts: `#!/bin/sh`, `set -euo pipefail`, quote all variables
- Kernel config: minimal fragment (only options differing from defconfig)
- Rootfs: BusyBox applets are symlinks, config files are plain text

## Testing

```bash
./test.sh --check              # Quick health check (direct kernel)
./test.sh --bios --check        # GRUB BIOS boot
./test.sh --keep-state --check  # With persistent state
```

## Issues

Include host OS, boot method, logs (`./test.sh --output=/tmp/boot.log`), and steps to reproduce.
