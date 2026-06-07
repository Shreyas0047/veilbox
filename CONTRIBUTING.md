# Contributing to Veilbox

Thanks for your interest in contributing! Veilbox is a bootable OS image built from a custom Linux kernel and BusyBox initramfs. This guide covers everything you need to get started.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [Project Architecture](#project-architecture)
- [Pull Request Guidelines](#pull-request-guidelines)
- [Coding Standards](#coding-standards)
- [Testing](#testing)
- [Issue Reporting](#issue-reporting)

## Code of Conduct

This project follows a **no-drama** policy. Be respectful, constructive, and assume good faith. Harassment, trolling, and personal attacks will not be tolerated.

## Getting Started

### Prerequisites

- **Fedora 40+** (or equivalent Linux distribution)
- **GCC 16+**, **make**, **binutils 2.46+**
- **QEMU 10+** (`qemu-system-x86_64`)
- **GRUB 2.x** (`grub2-tools`, `grub2-pc-modules`)
- **Git LFS** (for large artifact tracking)
- **Python 3** (for rootless GRUB installer)

### Clone

```bash
git lfs clone https://github.com/Shreyas0047/veilbox.git
cd veilbox
```

If you already cloned without LFS, run:

```bash
git lfs pull
```

### First Build

```bash
./build.sh           # Configure kernel, build rootfs, create disk image
MEM=2G ./test.sh     # Boot in QEMU
```

## Development Workflow

1. **Create a feature branch** from `main`:
   ```bash
   git checkout -b feat/your-feature
   ```

2. **Make changes** to the relevant files (see [Project Architecture](#project-architecture)).

3. **Rebuild and test**:
   ```bash
   ./build.sh          # Rebuilds kernel + rootfs + disk
   ./test.sh --check   # Quick health check
   ```

4. **Commit** with a descriptive message:
   ```bash
   git commit -m "feat: add your feature description"
   ```

5. **Push and open a PR**:
   ```bash
   git push -u origin feat/your-feature
   ```

## Project Architecture

### Build Pipeline (`build.sh`)

```
1. Kernel config  →  make defconfig + custom-os.config
2. Rootfs         →  BusyBox + services → initramfs
3. Disk image     →  partition + GRUB install + kernel copy
4. VDI            →  qemu-img convert
```

### Key Files

| File | Purpose |
|------|---------|
| `build.sh` | Full build pipeline (kernel, rootfs, disk, VDI) |
| `test.sh` | QEMU test runner with multiple boot modes |
| `rootfs/etc/inittab` | Init process configuration |
| `rootfs/etc/init.d/rcS` | System initialization script |
| `rootfs/etc/passwd` / `shadow` | User accounts and passwords |
| `rootfs/etc/dropbear/` | SSH host keys and authorized_keys |
| `rootfs/sbin/autologin` | Auto-login wrapper for CI/testing |
| `kernel/configs/custom-os.config` | Kernel configuration fragment |

### Adding a New Service

1. Add the binary to `rootfs/usr/bin/` or `rootfs/usr/sbin/`
2. Add its startup command to `rootfs/etc/init.d/rcS`
3. Add its shared libraries to `rootfs/lib64/` (check with `ldd`)
4. Rebuild and test: `./build.sh && MEM=2G ./test.sh --check`

### Modifying the Kernel

1. Edit `kernel/configs/custom-os.config` or use `make menuconfig`
2. Rebuild: `./build.sh` (only kernel step, ~2 min)

## Pull Request Guidelines

- **One feature per PR.** Small, focused changes are easier to review.
- **Rebase on `main`** before opening a PR.
- **Update the README** if your change affects usage or setup.
- **Include test results** in the PR description (e.g., `./test.sh --check` output).
- **Do not force-push** after a PR review has started unless requested.

### PR Title Format

```
feat:   new feature
fix:    bug fix
docs:   documentation changes
refactor: code restructure (no behavior change)
chore:  build, CI, or tooling changes
ci:     CI pipeline changes
```

### PR Description Template

```markdown
## Summary
Brief description of what this PR does.

## Changes
- List of changes
- Key files modified

## Testing
```
./test.sh --check
[OK]   VM booted successfully (login prompt detected)
```

## Checklist
- [ ] Code builds (`./build.sh`)
- [ ] Health check passes (`./test.sh --check`)
- [ ] Documentation updated (if needed)
- [ ] Commit messages follow conventions
```

## Coding Standards

### Shell Scripts (`build.sh`, `test.sh`, `rootfs/**/*.sh`)

- Use `#!/bin/sh` for portability (not `#!/bin/bash`)
- `set -euo pipefail` at the top of every script
- Quote all variable expansions: `"$VAR"`
- Use `snake_case` for variable names
- Error messages to stderr: `echo "error: ..." >&2`

### Kernel Config

- Keep `custom-os.config` as a minimal fragment (only options that differ from defconfig)
- Use `# CONFIG_FOO is not set` for explicitly disabled options
- Group related options with comments

### Rootfs

- BusyBox applets are symlinks to `/bin/busybox`
- Static binaries only (no external library dependencies when possible)
- Config files are plain text, no comments unless explaining non-obvious behavior

## Testing

### Quick Health Check

```bash
./test.sh --check
# [OK]   VM booted successfully (login prompt detected)
```

### Full Test Suite

```bash
# Direct kernel boot
./test.sh --check

# GRUB BIOS boot
./test.sh --bios --check

# With persistent state
./test.sh --keep-state --check

# With auto-login
./test.sh --autologin --check
```

### CI Pipeline

The health check script (`test.sh --check`) is designed to work as a GitHub Actions step. It exits 0 on success, 1 on failure, and has a built-in 45-second timeout. When combined with `--autologin`, it detects the shell prompt for faster and more reliable results.

## Issue Reporting

### Bug Reports

Include:

- **Environment:** Host OS, QEMU/VirtualBox version, RAM allocated
- **Boot method:** Direct kernel, GRUB BIOS, or VirtualBox
- **Logs:** Serial console output (use `./test.sh --output=/tmp/boot.log`)
- **Steps to reproduce:** What command you ran, what happened vs. expected

### Feature Requests

Describe:

- **What** you want to add or change
- **Why** it's useful (use case)
- **How** you think it should work

### Questions

Open a [discussion](https://github.com/Shreyas0047/veilbox/discussions) or an issue with the `question` label.
