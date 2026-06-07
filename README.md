<div align="center">
  <pre>
__      __  _ _ _               
\ \    / / (_) | |              
 \ \  / /__ _| | |__   _____  __
  \ \/ / _ \ | | '_ \ / _ \ \/ /
   \  /  __/ | | |_) | (_) >  < 
    \/ \___|_|_|_.__/ \___/_/\_\
  </pre>
  <h3>Lightweight bootable OS with embedded services</h3>
  <p>Custom Linux kernel · BusyBox · containerd · Dropbear SSH</p>
  <p>
    <a href="#-quick-start"><img src="https://img.shields.io/badge/-Quick%20Start-2ea44f?style=for-the-badge" alt="Quick Start"></a>
    <a href="#-getting-started"><img src="https://img.shields.io/badge/-Getting%20Started-1a73e8?style=for-the-badge" alt="Getting Started"></a>
    <a href="#%EF%B8%8F-ssh-access"><img src="https://img.shields.io/badge/-SSH%20Access-f7931e?style=for-the-badge" alt="SSH Access"></a>
    <a href="#-build-from-source"><img src="https://img.shields.io/badge/-Build%20From%20Source-6f42c1?style=for-the-badge" alt="Build From Source"></a>
  </p>
  <p>
    <img src="https://img.shields.io/badge/kernel-7.1.0--rc6-blue?style=flat-square&logo=linux" alt="Kernel">
    <img src="https://img.shields.io/badge/init-BusyBox-orange?style=flat-square&logo=alpinelinux" alt="Init">
    <img src="https://img.shields.io/badge/SSH-Dropbear-success?style=flat-square&logo=ssh" alt="SSH">
    <img src="https://img.shields.io/badge/container-runtime-containerd-important?style=flat-square&logo=docker" alt="Containerd">
    <img src="https://img.shields.io/badge/boot-GRUB%20BIOS-8892b0?style=flat-square" alt="GRUB">
    <img src="https://img.shields.io/github/license/Shreyas0047/veilbox?style=flat-square" alt="License">
    <img src="https://img.shields.io/github/repo-size/Shreyas0047/veilbox?style=flat-square" alt="Size">
  </p>
  <br>
</div>

---

## 📦 Quick Start

Clone it, import the VDI, boot. Under 30 seconds.

```bash
git lfs clone https://github.com/Shreyas0047/veilbox.git
cd veilbox
# Import output/veilbox.vdi into VirtualBox and boot
# Login: root / veiladmin
```

Want QEMU instead?

```bash
./test.sh               # Direct kernel boot (fast)
./test.sh --bios        # GRUB BIOS boot (full disk boot)
```

**Boot time:** Under 15 seconds from power-on to `veilbox login:`.

---

## ✨ Features

| Feature | Details |
|---------|---------|
| ⚡ **Custom Linux kernel** | v7.1.0-rc6, configured for VM and bare-metal |
| 🐚 **BusyBox shell** | Full init system, utilities, and POSIX shell |
| 🐳 **containerd + runc + nerdctl** | Container runtime ready out of the box |
| 🔑 **Dropbear SSH server** | Key-based and password authentication |
| 🖥️ **Dual console** | VGA (tty1) + serial (ttyS0) |
| 💾 **Persistent state** | Ext4 partition mounted at `/mnt/state` for data |
| 🎨 **Colored prompt** | Auto-displays guest IP on login |
| 🔌 **DHCP networking** | Works with QEMU SLiRP, VirtualBox NAT |
| 🚀 **Auto-login** | `veilbox.autologin` kernel param for CI/testing |
| 💿 **GRUB boot** | Full BIOS boot path for VirtualBox/bare-metal |

---

## 🖥️ Getting Started

### Prerequisites

- **Git LFS** (for cloning large files)
- **VirtualBox** (for the VDI) **or** **QEMU** (for direct boot/testing)
- **2 GB RAM** allocated to the VM

### Clone

```bash
git lfs clone https://github.com/Shreyas0047/veilbox.git
cd veilbox
```

> **Note:** Regular `git clone` downloads source only (150 KB). LFS files (VDI, kernel, SSH key) won't be available until you run `git lfs pull`.

### VirtualBox

1. **Create a new VM:**
   - Type: **Linux**
   - Version: **Linux 2.6 / 3.x / 4.x (64-bit)**
   - Memory: **2048 MB**
   - Hard disk: **Use an existing virtual hard disk file**
   - Select `output/veilbox.vdi`

2. **Optional — SSH port forwarding:**
   - Settings → Network → Advanced → **Port Forwarding**
   - Add: `SSH` | TCP | Host `2222` → Guest `22`

3. **Start the VM.** GRUB will appear, then the kernel boots, and you'll see:
   ```
   veilbox login: _
   ```

### QEMU

```bash
# Direct kernel boot (fastest, default)
./test.sh

# Full GRUB BIOS boot path
./test.sh --bios

# Auto-login as root (for automated workflows)
./test.sh --autologin

# Persistent state disk (data survives rebuilds)
./test.sh --keep-state

# Health check (exit 0 if booted)
./test.sh --check

# Custom memory
MEM=4G ./test.sh

# All together
./test.sh --keep-state --autologin --check
```

### VMware / Other Hypervisors

```bash
qemu-img convert -f raw -O vmdk output/veilbox.raw output/veilbox.vmdk
qemu-img convert -f raw -O qcow2 output/veilbox.raw output/veilbox.qcow2
```

### Bare Metal

```bash
sudo dd if=output/veilbox.raw of=/dev/sdX bs=4M status=progress
```

---

## 🔐 Credentials

| Method | Details |
|--------|---------|
| **Console login** | Username: `root` | Password: `veiladmin` |
| **SSH key** | `ssh -i output/ssh-test-key root@<ip>` |
| **SSH password** | `ssh root@<ip>` (password: `veiladmin`) |

---

## 🛠️ SSH Access

The guest runs a **Dropbear** SSH server on port 22. When using QEMU with default port forwarding, connect from the host via:

```bash
# Key-based authentication (recommended — no password prompt)
ssh -i output/ssh-test-key root@localhost -p 2222

# Password authentication
ssh root@localhost -p 2222
# Password: veiladmin
```

After login, you'll see a colored prompt showing the guest IP:

```
root@veilbox:~#
```

---

## 💾 State Persistence

| Mode | Data survives... | Description |
|------|-----------------|-------------|
| **Default** | VM reboots, not rebuilds | State stored on boot disk's `/state/` directory. `./build.sh --clean` wipes it. |
| **`--keep-state`** | VM reboots AND rebuilds | Creates `output/state-persist.img` (128MB ext4). Attached as a second virtual disk. |

```bash
# First run creates the persistent disk; subsequent runs reuse it
./test.sh --keep-state

# CI pipeline: persistent state + auto-login + health check
./test.sh --keep-state --autologin --check
```

---

## 🧩 Services

| Service | Status | Description |
|---------|--------|-------------|
| `init` (BusyBox) | ✅ | PID 1, runs rcS and respawns getty |
| `getty` (tty1) | ✅ | VGA console login |
| `getty` (ttyS0) | ✅ | Serial console login (supports auto-login) |
| `udhcpc` | ✅ | DHCP client, auto-configures networking |
| `dropbear` | ✅ | SSH server on port 22 |
| `containerd` | ✅ | Container runtime via gRPC |
| `nerdctl` | ✅ | Docker-compatible CLI for containerd |
| `runc` | ✅ | OCI-compliant container runtime |
| `syslogd` | ✅ | System logging |

---

## 🏗️ Build from Source

### Prerequisites

| Dependency | Minimum Version | Package (Fedora) |
|-----------|----------------|------------------|
| GCC | 16+ | `gcc` |
| GNU Make | 4.x | `make` |
| binutils | 2.46+ | `binutils` |
| QEMU | 10.x | `qemu-system-x86` |
| GRUB tools | 2.x | `grub2-tools grub2-pc-modules` |
| libfuse | 2.x | `fuse` (for squashfs) |
| genext2fs | — | `genext2fs` (for state disk) |

### Build

```bash
# Full build (~5 min)
./build.sh

# Clean rebuild
./build.sh --clean

# Boot after build
MEM=2G ./test.sh

# Health check
./test.sh --check
```

### What the build produces

| Artifact | Size | Description |
|----------|------|-------------|
| `output/vmlinuz` | 73 MB | bzImage kernel with embedded initramfs |
| `output/veilbox.raw` | 8 GB | Raw disk image (GRUB + kernel + state) |
| `output/veilbox.vdi` | 95 MB | VirtualBox disk image (sparse) |
| `output/ssh-test-key` | 400 B | ED25519 SSH private key for root access |
| `output/rootfs.squashfs` | 51 MB | XZ-compressed initramfs (for reference) |
| `output/state.img` | 128 MB | Legacy state disk |

---

## 📁 Disk Layout

```
veilbox.raw (8 GB, MBR)
└─ Partition 1 (ext4, label "VEILBOX", bootable)
   ├── /boot/vmlinuz          (kernel with embedded initramfs)
   ├── /boot/grub/grub.cfg    (GRUB config — serial + VGA)
   └── /state/                (persistent storage mounted at /mnt/state)
```

### Mount on Host

```bash
# Find partition offset
fdisk -l output/veilbox.raw

# Mount partition 1 (starts at sector 2048)
sudo mount -o loop,offset=$((2048*512)) output/veilbox.raw /mnt
```

---

## 📋 Project Structure

```
veilbox/
├── build.sh                  # Build script (kernel config + rootfs + disk)
├── test.sh                   # QEMU test runner with multiple boot modes
├── AGENTS.md                 # Development context and troubleshooting
├── CONTRIBUTING.md           # Guide for new contributors
├── README.md                 # This file
├── docs/                     # GitHub Pages website
│   └── index.html
├── kernel/
│   └── configs/
│       └── custom-os.config  # Kernel config fragment
├── rootfs/                   # Initramfs root filesystem
│   ├── init                  # /init -> /bin/busybox
│   ├── bin/                  # busybox binary
│   ├── sbin/                 # Utilities (getty, login, autologin, etc.)
│   ├── etc/                  # Configuration
│   │   ├── inittab
│   │   ├── init.d/rcS        # Init script
│   │   ├── passwd            # User accounts
│   │   ├── shadow            # Password hashes
│   │   ├── dropbear/         # SSH host keys + authorized_keys
│   │   └── containerd/       # containerd config
│   ├── root/                 # Root home directory
│   │   ├── .profile          # Colored prompt with IP display
│   │   └── .ssh/             # Root SSH keys
│   ├── usr/bin/              # containerd, nerdctl, runc, ctr
│   ├── usr/sbin/             # dropbear
│   └── lib64/                # Shared libraries
└── output/                   # Build artifacts
    ├── vmlinuz               # Kernel (LFS)
    ├── veilbox.raw           # Raw disk (git-ignored, too large)
    ├── veilbox.vdi           # VirtualBox disk (LFS)
    └── ssh-test-key          # SSH key (LFS)
```

---

## 🧪 CI / Health Check

The [`test.sh`](test.sh) script doubles as a CI health check:

```bash
./test.sh --check
# [OK]   VM booted successfully (login prompt detected)
```

This boots the VM (auto-enables autologin), waits up to 45 seconds, and confirms a shell prompt appeared. Exit code is 0 on success, 1 on failure. Ideal for:

- **GitHub Actions** workflows
- **Pre-commit hooks**
- **Automated testing pipelines**

---

## 🤝 Contributing

Contributions are welcome! See the [CONTRIBUTING.md](CONTRIBUTING.md) for:

- How to set up a development environment
- Coding and style guidelines
- Pull request process
- Reporting issues

---

## 🌐 Project Website

Visit the [Veilbox website](https://shreyas0047.github.io/veilbox) for a landing page with quick links, documentation, and downloads. Built with GitHub Pages from the `docs/` directory.

---

## 📖 Build Guide

A comprehensive **300+ page LaTeX PDF guide** is included in the repository:

**`veilbox-guide.pdf`** covers every aspect of the system:
- System architecture and design decisions
- Full build pipeline (14 stages, detailed)
- Kernel configuration reference (111+ options)
- GRUB bootloader internals (blocklist format, rootless install)
- Initramfs construction and kernel file list format
- Boot process (SeaBIOS → GRUB → kernel → userspace)
- QEMU virtualization configuration
- All services (containerd, Dropbear, udhcpc, syslogd)
- Troubleshooting guide for common issues
- Complete source code listings (build.sh, test.sh, configs)

```bash
# View the guide
open veilbox-guide.pdf
```

---

## 🐛 Troubleshooting

| Symptom | Likely Cause | Fix |
|---------|-------------|-----|
| VM shows black screen | Insufficient RAM | Allocate **2 GB+** (initramfs is ~90 MB) |
| GRUB "Read Error" | Corrupted blocklist in core.img | Run `./build.sh --clean` to rebuild |
| SSH "Connection refused" | Guest not on network | Log into console, run `ifconfig` to check IP |
| SSH password rejected | Dropbear v2025.89+ `-w` flag | Use `ssh -i output/ssh-test-key` (key auth) |
| Containerd won't start | State disk not writable | `mount | grep /mnt/state` to verify |
| Login prompt doesn't appear | Booting from wrong disk | Check VM boot order; ensure VEILBOX disk is first |

---

## 📄 License

This project is distributed under the [GNU General Public License v2](https://www.gnu.org/licenses/old-licenses/gpl-2.0.html).

---

<div align="center">
  <sub>
    Built with ❤️ for cybersecurity education and lab environments.
    <br>
    <a href="https://github.com/Shreyas0047/veilbox">GitHub</a> ·
    <a href="https://shreyas0047.github.io/veilbox">Website</a> ·
    <a href="CONTRIBUTING.md">Contributing</a>
  </sub>
</div>
