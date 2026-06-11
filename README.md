<div align="center">
  <pre>
__      __  _ _ _               
\ \    / / (_) | |              
 \ \  / /__ _| | |__   _____  __
  \ \/ / _ \ | | '_ \ / _ \ \/ /
   \  /  __/ | | |_) | (_) >  < 
    \/ \___|_|_|_.__/ \___/_/\_\
  </pre>
  <h3>Minimal bootable OS with embedded services</h3>
  <p>Custom Linux kernel · BusyBox · containerd · Dropbear SSH</p>
  <p>
    <a href="#-quick-start"><img src="https://img.shields.io/badge/-Quick%20Start-2ea44f?style=for-the-badge" alt="Quick Start"></a>
    <a href="#-features"><img src="https://img.shields.io/badge/-Features-1a73e8?style=for-the-badge" alt="Features"></a>
    <a href="#%EF%B8%8F-ssh-access"><img src="https://img.shields.io/badge/-SSH%20Access-f7931e?style=for-the-badge" alt="SSH Access"></a>
    <a href="#-build-from-source"><img src="https://img.shields.io/badge/-Build%20From%20Source-6f42c1?style=for-the-badge" alt="Build From Source"></a>
    <a href="#-build-guide"><img src="https://img.shields.io/badge/-Build%20Guide-8b5cf6?style=for-the-badge" alt="Build Guide"></a>
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

A **single-disk, bootable operating system** built from source: custom Linux kernel with embedded initramfs, BusyBox userspace, containerd container runtime, and Dropbear SSH. Boots in under 15 seconds to a working container host.

Clone → `git lfs pull` → `./test.sh` — under 30 seconds to the login prompt.

---

## 📦 Quick Start

### Prerequisites

- [Git LFS](https://git-lfs.com/) (for large binary artifacts)
- QEMU (for testing) or VirtualBox (for VM deployment)
- 2 GB RAM for the guest VM

### Clone & Boot

```bash
git clone https://github.com/Shreyas0047/veilbox.git
cd veilbox
git lfs pull                # Fetch pre-built kernel + disk image

# Direct kernel boot (fastest)
./test.sh

# GRUB BIOS boot (full boot path)
./test.sh --bios
```

Boot time: **under 15 seconds** to `veilbox login:` prompt.

### Log In

| Console | Username | Password |
|---------|----------|----------|
| VGA (tty1) / Serial (ttyS0) | `root` | `veiladmin` |
| SSH (port 2222 forwarded) | `root` | key or `veiladmin` |

---

## ✨ Features

| Feature | Details |
|---------|---------|
| **Custom Linux kernel** | v7.1.0-rc6, configured for VM and bare metal |
| **Embedded initramfs** | Root filesystem compiled into the kernel binary |
| **BusyBox userspace** | 300+ Unix utilities in a single binary |
| **containerd + runc + nerdctl** | Industry-standard container runtime, ready out of the box |
| **Dropbear SSH server** | Key-based and password authentication |
| **Dual console** | VGA text (tty1) + serial (ttyS0) |
| **Persistent state** | External ext4 disk mounted at `/mnt/state` |
| **DHCP networking** | Auto-configures via QEMU SLiRP / VirtualBox NAT |
| **Auto-login** | `veilbox.autologin` kernel param for CI workflows |
| **GRUB BIOS boot** | Full boot path for bare-metal or VirtualBox |
| **Rootless build** | Entire build runs without sudo |

---

## 🔐 Credentials

| Method | Command |
|--------|---------|
| **SSH key** | `ssh -i output/ssh-test-key root@localhost -p 2222` |
| **SSH password** | `ssh root@localhost -p 2222` (password: `veiladmin`) |

---

## 🖥️ QEMU

```bash
# Direct kernel boot (default, fastest)
./test.sh

# Full GRUB BIOS boot path
./test.sh --bios

# Auto-login for automated workflows
./test.sh --autologin

# Persistent state disk
./test.sh --keep-state

# Health check (exit 0 if booted)
./test.sh --check

# Custom RAM
MEM=4G ./test.sh

# To exit QEMU: Ctrl-A, then X (inside VM: shutdown)
```

---

## 🖥️ VirtualBox

```bash
# One-command setup and boot (creates VM, configures, starts)
./test.sh --vbox

# Register VM without starting
./test.sh --vbox-create

# Custom name / port
VM_NAME=veilbox SSH_PORT=2223 ./test.sh --vbox
```

**Manual setup:**
1. New VM → Name: `veilbox` → Type: `Linux` → Version: `Other Linux (64-bit)`
2. Memory: **2048 MB** (2 GB minimum)
3. Hard disk: *"Use an existing virtual hard disk file"* → `output/veilbox.vdi`
4. Settings → Network → Advanced → Port Forwarding → Add:
   - Host `2222` → Guest `22` (SSH)
   - Host `8080` → Guest `8080` (web containers)
5. Start the VM

SSH in: `ssh -i output/ssh-test-key root@localhost -p 2222`

---

## 🌐 Web Container Access

Port `8080` on your host is forwarded to port `8080` inside the VM (customize with `WEB_PORT=9090 ./test.sh`).

**Run a web server:**
```bash
# Inside the VM
nerdctl pull nginx:alpine
nerdctl run -d --name web -p 8080:80 nginx:alpine
```

Then visit **http://localhost:8080/** from your host browser.

**Custom port:**
```bash
# Host terminal
WEB_PORT=3000 ./test.sh

# Inside VM
nerdctl run -d -p 3000:80 nginx:alpine
```

---

## 🧩 Host Your Own Website

### Option A — Volume mount (quick, no build)
```bash
# Transfer your site to the VM
scp -P 2222 -r /path/to/website root@localhost:/mnt/state/site

# Inside the VM, serve with nginx
nerdctl pull nginx:alpine
nerdctl run -d --name website \
  --network host \
  -v /mnt/state/site:/usr/share/nginx/html:ro \
  nginx:alpine

# Update files anytime — changes are live instantly
scp -P 2222 new-index.html root@localhost:/mnt/state/site/
```

Visit **http://localhost:8080/** (or the `WEB_PORT` you set).

### Option B — Node.js app (containerized)
Apps with a backend (Express, Next.js, etc.) need Node.js:

```bash
# Transfer your app to the VM
scp -P 2222 -r /path/to/node-app root@localhost:/mnt/state/myapp

# Inside the VM, run it
nerdctl run -d --name myapp \
  --network host \
  -e PORT=8080 \
  -v /mnt/state/myapp:/app:ro \
  -w /app \
  node:26-slim \
  node server.js
```

**Why `--network host`?** The VM uses QEMU user-mode networking which doesn't support kernel bridge operations. `--network host` lets the container bind directly to the VM's network stack, and `-e PORT=8080` makes the app listen on the pre-forwarded port.

### Option C — Build a custom image (requires BuildKit)
If you've added BuildKit to the VM (see build.sh), you can build reusable images:

```dockerfile
FROM node:26-slim AS builder
WORKDIR /app
COPY package*.json ./
RUN apt-get update && apt-get install -y python3 make g++ && \
    npm ci --only=production && apt-get clean

FROM node:26-slim
WORKDIR /app
COPY --from=builder /app/node_modules ./node_modules
COPY . .
EXPOSE 3000
CMD ["node", "server.js"]
```

```bash
nerdctl build -t myapp:latest /mnt/state/myapp
nerdctl run -d --name myapp --network host -e PORT=8080 myapp:latest
```

---

## 🏗️ Build from Source

The entire OS can be built from source with no sudo required:

```bash
# Full build (≈5 min)
./build.sh

# Clean rebuild
./build.sh --clean

# Boot after build
MEM=2G ./test.sh
```

### Build Artifacts

| Artifact | Size | Description |
|----------|------|-------------|
| `output/vmlinuz` | 73 MB | bzImage kernel with embedded initramfs (LFS) |
| `output/veilbox.raw` | 8 GB | Raw disk image (GRUB + kernel + state) |
| `output/veilbox.vdi` | 95 MB | VirtualBox sparse disk image (LFS) |
| `output/ssh-test-key` | 400 B | ED25519 SSH private key (LFS) |

### Build Requirements

| Dependency | Version | Package (Fedora) |
|-----------|---------|------------------|
| GCC | 16+ | `gcc` |
| GNU Make | 4.x | `make` |
| QEMU | 10.x | `qemu-system-x86` |
| GRUB tools | 2.x | `grub2-tools` |
| CPIO | 2.13+ | `cpio` |

---

## 📖 Build Guide

A **152-page LaTeX technical reference** is included:

**`veilbox-guide.pdf`** covers the complete system:

| Chapter | Topics |
|---------|--------|
| 1–2 | Overview, System Architecture (8-layer model) |
| 3–5 | Prerequisites, Build Pipeline, Kernel Configuration |
| 6–8 | GRUB Bootloader, Initramfs, Boot Process |
| 9–11 | QEMU Virtualization, Services, Troubleshooting |
| 12–15 | Kernel Build System, BusyBox, Init, Containerd |
| 16–19 | Dropbear SSH, Networking, Filesystem Layers, Security |
| 20–22 | Performance Tuning, Container Workloads, Comparisons |
| 23–24 | Tutorials, Quick Reference |
| 25 | Appendix — Full source code listings |

```bash
open veilbox-guide.pdf    # macOS
xdg-open veilbox-guide.pdf # Linux
```

---

## 📁 Repository Structure

```
veilbox/
├── build.sh                  # Build pipeline (14 stages)
├── test.sh                   # QEMU / VirtualBox test runner
├── CONTRIBUTING.md           # Contributor guide
├── README.md                 # This file
├── .gitignore                # Excludes kernel source & build artifacts
├── .gitattributes            # Git LFS for large binaries
├── veilbox-guide.tex         # LaTeX source for the build guide
├── veilbox-guide.pdf         # 152-page technical reference
├── kernel/
│   └── configs/
│       └── custom-os.config  # Kernel config fragment (111+ options)
├── rootfs/                   # Initramfs source tree
│   ├── bin/                  # Symlinks to BusyBox
│   ├── sbin/                 # System utilities (autologin, shutdown)
│   ├── etc/                  # Configuration (inittab, rcS, passwd, etc.)
│   ├── opt/cni/bin/          # CNI plugins (bridge, host-local, loopback)
│   ├── root/                 # Root profile with colored prompt
│   └── usr/share/udhcpc/     # DHCP client script
├── chapters/                 # LaTeX chapter source files (25 chapters)
├── output/                   # Build artifacts (LFS tracked)
│   ├── vmlinuz               # Pre-built kernel (~73 MB)
│   ├── veilbox.vdi           # VirtualBox disk image (~95 MB)
│   └── ssh-test-key          # SSH identity key
└── docs/                     # GitHub Pages website
```

**Note:** Only source files are committed. Binaries (containerd, nerdctl, runc, dropbear, BusyBox, shared libraries) are downloaded by `build.sh` at build time. Pre-built artifacts are distributed via Git LFS.

---

## 🤝 Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on setting up a development environment, coding standards, and the PR process.

---

## 📄 License

[GNU General Public License v2](https://www.gnu.org/licenses/old-licenses/gpl-2.0.html)

---

<div align="center">
  <sub>
    Built for cybersecurity education and lab environments.
    <br>
    <a href="https://github.com/Shreyas0047/veilbox">GitHub</a> ·
    <a href="https://shreyas0047.github.io/veilbox">Website</a> ·
    <a href="CONTRIBUTING.md">Contributing</a>
  </sub>
</div>
