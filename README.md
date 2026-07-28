<div align="center">

  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="branding/logo.svg">
    <img alt="Veilbox" src="branding/logo.svg" width="120" height="120">
  </picture>

  <h1>Veilbox Linux</h1>

  <p><strong>A live Linux distribution for DevOps engineers</strong></p>
  <p><em>Niri scrollable-tiling compositor · Noctalia shell · Container-native toolchain · Live ISO</em></p>

  <p>
    <a href="#-features">
      <img alt="Features" src="https://img.shields.io/badge/Features-8A2BE2?style=flat-square">
    </a>
    <a href="#-devops-toolchain">
      <img alt="DevOps" src="https://img.shields.io/badge/DevOps-20%2B%20tools-00C853?style=flat-square">
    </a>
    <a href="#-specifications">
      <img alt="Debian Trixie" src="https://img.shields.io/badge/Debian-Trixie-CC3333?style=flat-square&logo=debian">
    </a>
    <a href="#-download">
      <img alt="Download" src="https://img.shields.io/github/v/release/Shreyas0047/veilbox?style=flat-square&color=blue">
    </a>
    <a href="#-build-from-source">
      <img alt="Build" src="https://img.shields.io/badge/Build-live--build-2196F3?style=flat-square">
    </a>
  </p>

  <p>
    <img alt="Kernel" src="https://img.shields.io/badge/Kernel-6.12%20amd64-2666CC?style=flat-square">
    <img alt="Compositor" src="https://img.shields.io/badge/WM-Niri-6A0DAD?style=flat-square">
    <img alt="Shell" src="https://img.shields.io/badge/Shell-Noctalia-FF6B6B?style=flat-square">
    <img alt="Installer" src="https://img.shields.io/badge/Installer-Calamares-4CAF50?style=flat-square">
    <img alt="Container" src="https://img.shields.io/badge/Container-Docker-2496ED?style=flat-square&logo=docker">
    <img alt="ISO" src="https://img.shields.io/badge/ISO-1.7%20GB-FF5722?style=flat-square">
    <img alt="CI" src="https://img.shields.io/github/actions/workflow/status/Shreyas0047/veilbox/build.yml?branch=main&style=flat-square&logo=github">
  </p>

  <br>

</div>

Veilbox is a live Linux distribution purpose-built for DevOps engineers. Boot it on any x86_64 machine and instantly get a keyboard-driven Wayland desktop with a complete container-native DevOps toolchain — Docker, Kubernetes tools, Terraform, cloud CLIs, and system monitoring — pre-installed and ready to work.

No installation required. No configuration needed. Boot the ISO, and you're in a scrollable-tiling Niri compositor with Noctalia shell, terminal, launcher, and your full toolkit. When you're ready to make it permanent, the Calamares graphical installer handles disk installation.

Built weekly via GitHub Actions on Debian Trixie (13). Vetted with automated builds, size checks, and release publishing.

---

## Why Veilbox for DevOps?

| Need | How Veilbox Delivers |
|------|---------------------|
| **Zero-setup workstation** | Live ISO — burn to USB, boot, and you have a full environment in 30 seconds |
| **Ephemeral / disposable** | Use it for CTF work, audits, CI debugging, or conference machines. Reboot to reset |
| **Container-native toolkit** | Docker, kubectl, Helm, k9s, kind, kustomize, stern, nerdctl — all pre-installed |
| **Cloud CLIs** | AWS CLI, GitHub CLI, Terraform, Ansible — ready for any cloud |
| **Keyboard-driven** | Niri scrollable-tiling compositor + Noctalia shell = no mouse needed |
| **Multi-monitor fluid** | Niri's scrollable workspace model handles any display arrangement |
| **Serial console friendly** | Auto-login on ttyS0 (115200 baud) — drop into a VM, get a shell instantly |
| **GitHub Actions CI** | Every commit builds and releases a fresh ISO. Weekly cron keeps packages current |
| **Auditable & free** | Open source GPL-2.0 build scripts. Inspect, fork, customize anything |

### When to Choose Veilbox

- Your daily driver is a terminal, not a mouse
- You want Docker + k8s tooling without installing anything
- You need a disposable Linux environment for CI troubleshooting or security work
- You want Niri's scrollable-tiling on a modern Debian base, pre-configured

### Limitations vs. Cloud Linux

Veilbox is a **desktop OS for DevOps engineers** — not a server OS. Compared to commercial cloud Linux distributions:

- **No LTS guarantee** — Debian Trixie is rolling/testing; stable releases are periodic
- **No live patching** — kernel updates require reboot
- **No marketplace presence** — not on AWS/Azure/GCP; use it on any VM by attaching the ISO
- **No vendor support** — community-driven, no escalation channel

Use it where you need a powerful, ephemeral desktop workstation. For production servers, see [veilbox-cloud](https://github.com/Shreyas0047/veilbox-cloud).

---

## Quick Start

### Download

Grab the latest ISO from [GitHub Releases](https://github.com/Shreyas0047/veilbox/releases):

```bash
# Download and write to USB
wget https://github.com/Shreyas0047/veilbox/releases/latest/download/veilbox-2.0-amd64.iso
dd if=veilbox-2.0-amd64.iso of=/dev/sdX bs=4M status=progress
```

### Boot in QEMU

```bash
qemu-system-x86_64 -m 2048 -smp 2 -enable-kvm \
    -cdrom output/veilbox-2.0-amd64.iso \
    -netdev user,id=net0,hostfwd=tcp::2224-:22 \
    -device virtio-net,netdev=net0 \
    -nographic -serial mon:stdio
# Login: veilbox / veilbox (passwordless sudo)
```

---

## Features

| Area | Description |
|---|---|
| **Base OS** | Debian Trixie (13) — rolling snapshot, wide hardware support via live-build |
| **Compositor** | [Niri](https://github.com/niri-wm/niri) — scrollable-tiling Wayland compositor, fluid multi-monitor workflow |
| **Desktop Shell** | [Noctalia](https://noctalia.dev) — keyboard-driven shell, auto-launched by niri |
| **Container Runtime** | Docker CE + containerd + Docker Compose + nerdctl |
| **Orchestration CLI** | kubectl, Helm, k9s, stern, kind, kustomize |
| **Infrastructure as Code** | Terraform, Ansible |
| **Cloud CLIs** | AWS CLI v2, GitHub CLI |
| **Utilities** | yq, jq, dive, skopeo, PipeWire audio |
| **Installer** | Calamares graphical installer for permanent installs |
| **Keyboard-centric** | Fuzzel launcher (Mod+D), foot terminal (Mod+Return), full Niri keybindings |
| **Display fallback** | xwayland-satellite, Xterm fallback, serial console auto-login |
| **Session watchdog** | Niri crash -> text console fallback with diagnostic logging |

---

## Specifications

| Component | Detail |
|---|---|
| **Base OS** | Debian Trixie (13) — amd64 |
| **Kernel** | `linux-image-amd64` (6.12) — full desktop kernel |
| **Compositor** | [Niri](https://github.com/niri-wm/niri) 26.04 — community `.deb` |
| **Shell** | [Noctalia](https://noctalia.dev) v5 — official APT repo |
| **Launcher** | [Fuzzel](https://codeberg.org/dnkl/fuzzel) — Wayland-native |
| **Terminal** | [Foot](https://codeberg.org/dnkl/foot) + XTerm — Wayland + X11 fallback |
| **Notifications** | [Mako](https://github.com/emersion/mako) — Wayland notification daemon |
| **Audio** | PipeWire + WirePlumber + ALSA compatibility |
| **Container** | Docker CE + containerd + Docker Compose plugin |
| **Installer** | Calamares 3.3 (graphical, binary-only on ISO) |
| **Init** | systemd |
| **Bootloader** | ISOLINUX + GRUB (BIOS + UEFI) |
| **Image Size** | ~1.7 GB, XZ-compressed squashfs |

### Kernel Command Line

```
boot=live components quiet splash console=tty0 console=ttyS0,115200n8
username=veilbox locales=en_US.UTF-8 keyboard-layouts=us timezone=UTC
```

---

## DevOps Toolchain

### Container & Orchestration

| Tool | Source | Purpose |
|---|---|---|
| **Docker CE** | `get.docker.com` | Container runtime |
| **Docker Compose** | Docker plugin | Multi-container orchestration |
| **containerd** | Docker bundle | Container runtime daemon |
| **nerdctl** | GitHub Releases | containerd CLI |
| **kubectl** | GitHub Releases | Kubernetes CLI |
| **Helm** | GitHub Releases | Kubernetes package manager |
| **k9s** | GitHub Releases | Kubernetes TUI dashboard |
| **stern** | GitHub Releases | Multi-pod log tailing |
| **kind** | GitHub Releases | Local Kubernetes clusters |
| **kustomize** | GitHub Releases | Kubernetes config management |
| **skopeo** | Debian repo | Container image inspection |
| **dive** | GitHub Releases | Docker layer inspector |

### Infrastructure as Code & Cloud

| Tool | Source | Purpose |
|---|---|---|
| **Terraform** | HashiCorp APT | Infrastructure provisioning |
| **Ansible** | Debian repo | Configuration management |
| **AWS CLI v2** | AWS installer | Amazon Web Services |
| **GitHub CLI** | GitHub APT | GitHub operations |

### Utilities

| Tool | Source | Purpose |
|---|---|---|
| **yq** | GitHub Releases | YAML/JSON processor |
| **jq** | Debian repo | JSON processor |
| **fuzzel** | Debian repo | App launcher |
| **foot** | Debian repo | Wayland terminal |
| **xterm** | Debian repo | X11 terminal (fallback) |
| **mako** | Debian repo | Notification daemon |
| **grim / slurp** | Debian repo | Screenshot tools |
| **PipeWire / WirePlumber** | Debian repo | Audio |

---

## Architecture

```
veilbox/
├── auto/config                 # live-build configuration
├── build.sh                     # Build wrapper (clean, config, build, qemu)
├── docker-build.sh              # Containerized build wrapper
├── Dockerfile.build            # Builder container (Python + live-build)
├── config/
│   ├── package-lists/           # APT package manifests
│   │   ├── base.list.chroot
│   │   ├── desktop.list.chroot
│   │   ├── devops.list.chroot
│   │   └── live.list.chroot
│   ├── hooks/
│   │   ├── live/                # Runtime chroot hooks
│   │   │   ├── branding.hook.chroot
│   │   │   ├── cleanup.hook.chroot
│   │   │   ├── devops-tools.hook.chroot
│   │   │   ├── docker-install.hook.chroot
│   │   │   ├── noctalia-install.hook.chroot
│   │   │   └── user-setup.hook.chroot
│   │   └── normal/
│   │       └── patch-isolinux-timeout.binary
│   ├── includes.chroot/         # Filesystem overlay
│   │   ├── etc/
│   │   │   ├── xdg/niri/        # Niri compositor config
│   │   │   ├── calamares/       # Installer modules
│   │   │   ├── motd
│   │   │   ├── update-motd.d/
│   │   │   ├── skel/
│   │   │   └── systemd/
│   │   ├── home/veilbox/
│   │   ├── usr/local/bin/
│   │   └── boot/grub/
│   └── includes.binary/
├── branding/
├── calamares/
└── output/
```

---

## Build from Source

### Prerequisites

- Linux host with `podman` or `docker`
- ~10 GB free disk space
- Network access to Debian mirrors and GitHub/cloud provider releases

### Quick Build (Containerized)

```bash
git clone https://github.com/Shreyas0047/veilbox.git
cd veilbox
./docker-build.sh build
# Output: output/veilbox-2.0-amd64.iso
```

### Native Build

```bash
sudo apt install live-build
./auto/config
sudo lb clean
sudo lb build
```

### Build Commands

| Command | Description |
|---|---|
| `./build.sh clean` | Clean build artifacts |
| `./build.sh config` | Re-run `lb config` |
| `./build.sh build` | Build the ISO |
| `./build.sh all` | Clean + config + build |
| `./build.sh qemu` | Boot the ISO in QEMU |

---

## Usage

### Live Session

The ISO boots to GRUB (auto-selects after 5 seconds):

1. **Boots** with Plymouth splash and quiet kernel parameters
2. **Auto-logs in** as `veilbox` on the display (tty1) and serial console (ttyS0)
3. **Starts Niri** via `niri-session` wrapper
4. **Launches** Noctalia shell, Mako notifications, foot terminal

### Credentials

| Field | Value |
|---|---|
| Username | `veilbox` |
| Password | `veilbox` |
| sudo | Passwordless (`NOPASSWD: ALL`) |

### Desktop Keybindings

| Shortcut | Action |
|---|---|
| Mod+D | Open fuzzel launcher |
| Mod+Return | Open foot terminal |
| Mod+Q | Close focused window |
| Mod+F | Toggle fullscreen |
| Mod+H / L | Focus column left / right |
| Mod+Shift+H / L | Move column |
| Mod+1-9 | Switch to workspace |
| Mod+Shift+1-9 | Move window to workspace |
| Mod+Shift+S | Screenshot region |
| Print | Fullscreen screenshot |

### Session Recovery

If Niri crashes, the `niri-session` watchdog restores the text console with diagnostic logs at `/tmp/veilbox/`.

### Install to Disk

```bash
pkexec calamares -d
```

---

## License

Veilbox is built on Debian GNU/Linux and the Linux kernel. Build scripts, configuration, and documentation in this repository are licensed under **GNU General Public License v2.0**.

---

<div align="center">
  <sub>Built on Debian Live Build · Kernel 6.12 · Niri · Noctalia · Docker<br>
  <a href="https://github.com/Shreyas0047/veilbox">github.com/Shreyas0047/veilbox</a></sub>
</div>
