<div align="center">

  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="branding/logo.svg">
    <img alt="Veilbox" src="branding/logo.svg" width="120" height="120">
  </picture>

  <h1>Veilbox</h1>

  <p><strong>Minimal · Container-native · DevOps-ready</strong></p>

  <p>
    <a href="#-features">
      <img alt="Features" src="https://img.shields.io/badge/Features-8A2BE2?style=flat-square">
    </a>
    <a href="#-included-tooling">
      <img alt="DevOps" src="https://img.shields.io/badge/DevOps-20%2B%20tools-00C853?style=flat-square">
    </a>
    <a href="#-specifications">
      <img alt="Debian Trixie" src="https://img.shields.io/badge/Debian-Trixie-CC3333?style=flat-square&logo=debian">
    </a>
    <a href="#-build-from-source">
      <img alt="Build" src="https://img.shields.io/badge/Build-live--build-2196F3?style=flat-square">
    </a>
    <a href="#-license">
      <img alt="License GPL-2.0" src="https://img.shields.io/badge/License-GPL%202.0-FF6F00?style=flat-square">
    </a>
  </p>

  <p>
    <img alt="Kernel" src="https://img.shields.io/badge/Kernel-6.12%20amd64-2666CC?style=flat-square">
    <img alt="Compositor" src="https://img.shields.io/badge/WM-Niri-6A0DAD?style=flat-square">
    <img alt="Shell" src="https://img.shields.io/badge/Shell-Noctalia-FF6B6B?style=flat-square">
    <img alt="Installer" src="https://img.shields.io/badge/Installer-Calamares-4CAF50?style=flat-square">
    <img alt="Container" src="https://img.shields.io/badge/Container-Docker-2496ED?style=flat-square&logo=docker">
    <img alt="ISO" src="https://img.shields.io/badge/ISO-2.0%20GB-FF5722?style=flat-square">
  </p>

  <br>

  <p><em>A lightweight Debian-based live distribution with the <a href="https://github.com/niri-wm/niri">Niri</a> scrolling compositor, <a href="https://noctalia.dev">Noctalia</a> shell, and a full DevOps toolchain — packaged as a bootable live ISO with Calamares installer.</em></p>

  <br>

  <pre>
               .                    .
            .   *  .  _  .  *   .
         .    .    /_\    .    .
   .   *    .   . /___\ .   .    *   .
     .    .    __/_____\__    .    .
  .     .  .-'           '-.  .     .
         /                 \
  .    . |   V E I L B O X | .    .
        |    v2  (Trixie)   |
  .    . \                 / .    .
         '-._           _.-'
  .    *     '---------+      *    .
         .     |  ||  |     .
      *     .  |  ||  |  .     *
            .  |__||__|  .
               |________|
  </pre>

</div>

---

## Features

- **Debian Trixie base** — Built with `live-build`, using the full `linux-image-amd64` kernel with all GPU/driver support
- **Niri compositor** — Scrollable-tiling Wayland compositor for a unique multi-monitor workflow, with Noctalia auto-started as the shell
- **Noctalia shell** — Minimal, keyboard-driven shell environment (v5 beta) auto-launched via `spawn-at-startup` in the niri config
- **Docker CE** — Container runtime pre-installed via `get.docker.com`
- **DevOps toolchain** — 20+ pre-installed tools: Helm, Terraform, Ansible, AWS CLI, Azure CLI, GitHub CLI, kubectl*, and more
- **Calamares installer** — Graphical installer for persistent installations
- **Systemd init** — Modern init system with NetworkManager, SSH, and Docker socket management
- **Auto-login** — Boots directly into the Niri+Noctalia desktop on tty1
- **Full hardware support** — Desktop kernel with Intel/AMD/NVIDIA GPU drivers, XWayland, PipeWire audio, Bluetooth, power management
- **Custom branding** — Dark-themed GRUB menu, Veilbox wallpaper, ASCII MOTD, colored bash prompt with aliases

> \* kubectl and trivy are attempted during build; skipped if GitHub releases are unreachable (DNS/cert issues in chroot)

---

## Specifications

| Component | Detail |
|---|---|
| **Base OS** | Debian Trixie (13) — amd64 |
| **Kernel** | `linux-image-amd64` (6.12.y) — Debian full desktop kernel (i915, amdgpu, nouveau, all GPU/audio/network drivers) |
| **Compositor** | [Niri](https://github.com/niri-wm/niri) — community `.deb` from `Alexvs159/niri-debian` |
| **Shell/Panel** | [Noctalia](https://noctalia.dev) v5 — official APT repo `pkg.noctalia.dev/apt` |
| **Launcher** | Fuzzel (Wayland-native app launcher) |
| **Terminal** | Foot (Wayland-native terminal emulator) |
| **Notifications** | Mako (Wayland-native notification daemon) |
| **Audio** | PipeWire + WirePlumber |
| **Clipboard** | wl-clipboard (Wayland clipboard utilities) |
| **Screenshots** | grim + slurp (Wayland screenshot/region tools) |
| **Container** | Docker CE (via convenience script) + containerd |
| **Installer** | Calamares 3.3 (graphical, binary-only on ISO) |
| **Init** | systemd |
| **Bootloader** | ISOLINUX + GRUB (Legacy + EFI) |
| **Image** | Live ISO, ~2.0 GB, XZ-compressed squashfs |

### Boot Parameters (default)

```
boot=live components quiet splash
username=veilbox
locales=en_US.UTF-8
keyboard-layouts=us
timezone=UTC
```

---

## Included Tooling

### DevOps & Cloud

| Tool | Source | Notes |
|---|---|---|
| **kubectl** | GitHub Releases (kubernetes/kubernetes) | Skipped if DNS unavailable |
| **Helm** | GitHub Releases (helm/helm) | |
| **Terraform** | HashiCorp APT repo | |
| **Ansible** | Debian repo | Core, no playbooks |
| **yq** | GitHub Releases (mikefarah/yq) | YAML processor |
| **dive** | GitHub Releases (wagoodman/dive) | Docker image layer inspector |
| **k9s** | GitHub Releases (derailed/k9s) | Kubernetes TUI |
| **stern** | GitHub Releases (stern/stern) | Multi-pod log tailing |
| **kind** | GitHub Releases (kubernetes-sigs/kind) | Local Kubernetes clusters |
| **minikube** | GitHub Releases (kubernetes/minikube) | Local Kubernetes |
| **kustomize** | GitHub Releases (kubernetes-sigs/kustomize) | Kubernetes config management |
| **argocd** | GitHub Releases (argoproj/argo-cd) | Argo CD CLI |
| **GitHub CLI** | GitHub APT repo (`cli.github.com/packages`) | |
| **AWS CLI v2** | AWS installer bundle | |
| **Azure CLI** | Microsoft APT repo | |
| **skopeo** | Debian repo | Container image inspection |
| **jq** | Debian repo | JSON processor |

### System

| Package | Purpose |
|---|---|
| `systemd` + `systemd-sysv` | Init system |
| `NetworkManager` | Network management |
| `openssh-server` + `openssh-client` | SSH access |
| `pipewire` + `pipewire-pulse` + `wireplumber` | Audio |
| `plymouth` | Boot splash |
| `cryptsetup` | Disk encryption |
| `grub-pc` + `grub-efi-*` + `shim-signed` | Bootloader (Legacy + UEFI Secure Boot) |
| `firmware-linux` + `firmware-sof-signed` | Hardware firmware |
| `intel-microcode` + `amd64-microcode` | CPU microcode |
| `bluez` | Bluetooth stack |
| `upower` + `power-profiles-daemon` | Power management |
| `polkit-kde-agent-1` | PolicyKit authentication agent |

### Desktop

| Package | Purpose |
|---|---|
| `foot` | Wayland terminal emulator |
| `fuzzel` | Wayland app launcher |
| `mako-notifier` | Notification daemon |
| `wl-clipboard` | Clipboard utilities |
| `grim` + `slurp` | Screenshot + region selection |
| `xdg-desktop-portal` + `xdg-desktop-portal-gtk` + `xdg-desktop-portal-wlr` | Desktop portals |
| `xwayland` + `xserver-xorg-core` | X11 app compatibility |
| `xserver-xorg-video-all` + `xserver-xorg-input-all` | Xorg GPU + input drivers |
| `mesa-va-drivers` | VA-API video acceleration |
| `pipewire-alsa` | ALSA audio compatibility |
| `bluez` | Bluetooth stack |
| `upower` + `power-profiles-daemon` | Power management |
| `polkit-kde-agent-1` | PolicyKit authentication agent |
| `fonts-font-awesome`, `fonts-noto*`, `fonts-noto-color-emoji`, `fonts-liberation` | Fonts |
| `adwaita-icon-theme` + `papirus-icon-theme` | Icon themes |

### Calamares Installer

The ISO includes Calamares (binary-only, not installed to target) for installing Veilbox to a hard drive. Modules configured:

- `welcome` — System requirements check
- `locale` — Timezone and locale selection
- `keyboard` — Keyboard layout
- `partition` — Disk partitioning
- `users` — User creation
- `summary` — Install summary
- `grubcfg` — GRUB configuration
- `bootloader` — Bootloader installation
- `finished` — Completion screen

---

## Architecture

```
veilbox/
├── auto/config              # live-build configuration
├── build.sh                 # Build wrapper (clean, config, build, qemu)
├── docker-build.sh          # Host-side wrapper for podman/docker
├── Dockerfile.build         # Python + live-build container image
├── config/
│   ├── package-lists/       # APT package lists
│   │   ├── base.list.chroot
│   │   ├── desktop.list.chroot
│   │   ├── devops.list.chroot
│   │   ├── calamares.list.binary
│   │   └── live.list.chroot
│   ├── hooks/
│   │   ├── live/            # Chroot hooks (runtime)
│   │   │   ├── branding.hook.chroot
│   │   │   ├── cleanup.hook.chroot
│   │   │   ├── devops-tools.hook.chroot
│   │   │   ├── docker-install.hook.chroot
│   │   │   ├── noctalia-install.hook.chroot
│   │   │   └── user-setup.hook.chroot
│   │   └── normal/          # Normal hooks + binary hooks
│   │       └── patch-isolinux-timeout.binary
│   ├── includes.chroot/     # Filesystem overlay
│   │   ├── etc/
│   │   │   ├── xdg/niri/    # Niri compositor config (Noctalia spawn, keybindings)
│   │   │   ├── calamares/   # Calamares configuration
│   │   │   ├── motd         # Message of the day
│   │   │   ├── update-motd.d/
│   │   │   ├── skel/        # Skeleton (.bashrc, .config/niri/)
│   │   │   ├── default/     # Default config files
│   │   │   └── systemd/     # Systemd drop-ins (auto-login)
│   │   ├── home/veilbox/    # User home directory (.bash_profile)
│   │   ├── usr/             # Scripts, wallpapers, wayland-sessions
│   │   └── boot/grub/       # GRUB theme
│   └── includes.binary/     # Binary-stage includes
├── branding/                # Source assets (logo, wallpaper, GRUB theme)
├── calamares/               # Calamares branding (branding.desc, show.qml)
└── output/                  # Build output (veilbox-2.0-amd64.iso)
```

---

## Build from Source

### Prerequisites

- Linux host with `podman` or `docker`
- ~10 GB free disk space
- Network access to Debian mirrors + GitHub releases

### Quick Build

```bash
# Clone the repository
git clone https://github.com/veilbox/linux.git
cd linux

# Build the builder container and run the ISO build
./docker-build.sh build
```

The ISO will be written to `output/veilbox-2.0-amd64.iso`.

### Step-by-Step

```bash
# 1. Build the builder container
podman build -t veilbox-builder -f Dockerfile.build .

# 2. Run the build (this runs `lb build` inside the container)
podman run --rm --privileged \
    -v "$(pwd):/repo:Z" \
    veilbox-builder \
    "cd /repo && ./build.sh all"

# 3. The ISO is at output/veilbox-2.0-amd64.iso
ls -lh output/
```

### Build Commands

| Command | Description |
|---|---|
| `./build.sh clean` | Remove build artifacts |
| `./build.sh config` | Re-run `lb config` |
| `./build.sh build` | Build the ISO |
| `./build.sh all` | Clean + config + build |
| `./build.sh qemu` | Boot ISO in QEMU |

### Testing in QEMU

```bash
# Boot with graphical display
./build.sh qemu

# Or manual boot with kernel passthrough
qemu-system-x86_64 -m 4096 -smp 4 -enable-kvm \
    -cdrom output/veilbox-2.0-amd64.iso \
    -kernel /live/vmlinuz -initrd /live/initrd.img \
    -append "boot=live components quiet splash console=ttyS0" \
    -nographic -serial mon:stdio
```

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `ARCH` | `amd64` | Target architecture |
| `MIRROR` | `http://deb.debian.org/debian` | Debian mirror |
| `OUTPUT_DIR` | `./output` | ISO output directory |
| `PARALLEL` | `nproc` | Parallel build jobs |
| `BUILD_CMD` | `build` | Build command for docker-build.sh |

---

## Live System Details

### Default User

- **Username:** `veilbox`
- **Password:** `veilbox`
- **Sudo:** Passwordless (`NOPASSWD: ALL`)

### Auto-Login

The live ISO auto-logs in as `veilbox` on tty1 and starts the Niri desktop session via a systemd drop-in override:

```
/etc/systemd/system/getty@tty1.service.d/autologin.conf
```

### Desktop Session

The `.bash_profile` for `veilbox` detects tty1 and launches `niri-session`, which starts `pipewire` and `wireplumber`, then `exec niri`. Once niri loads, it reads `/etc/xdg/niri/config.kdl` and auto-starts:

1. `xwayland-satellite` — X11 app compatibility layer
2. `noctalia` — Shell/panel bar
3. `mako` — Desktop notification daemon

The niri configuration includes:
- Noctalia auto-launched as the shell/panel (`spawn-at-startup "noctalia"`)
- Fuzzel as the app launcher (Mod+D)
- Foot as the default terminal (Mod+Enter)
- Mako for desktop notifications
- Workspace switching (Mod+1-9) and window movement (Mod+Shift+1-9)
- Consume-or-expand-window (Mod+Space)
- Screenshot with grim+slurp (Mod+Shift+S) or fullscreen (Print)
- Fullscreen toggle (Mod+F)
- Column-based window navigation (Mod+H/L/Up/Down)
- Touchpad tap-to-click and natural scrolling
- Wayland environment variables (QT_QPA_PLATFORM, GDK_BACKEND, MOZ_ENABLE_WAYLAND)

### Networking

- DHCP-enabled via NetworkManager
- SSH server running on port 22 (password auth)
- Docker socket listening on `/var/run/docker.sock`

### Boot Process

1. ISOLINUX loads and auto-selects the `live-cloud-amd64` entry (5-second timeout)
2. Kernel boots with `linux-image-amd64` (full desktop kernel with all GPU/driver support)
3. Initramfs detects the squashfs on the live media
4. Systemd initializes with live-config for user/locale setup
5. Getty auto-logs in as `veilbox` on tty1
6. `.bash_profile` launches `niri-session`
7. Niri compositor starts with Noctalia shell

---

## License

Veilbox is built on Debian GNU/Linux and the Linux kernel. The build scripts and configuration files in this repository are licensed under **GNU General Public License v2.0**.

See [`COPYING`](COPYING) for details.

---

<div align="center">
  <sub>Built with ❄️ using Debian Live Build · Kernel 6.12 · Niri · Noctalia</sub>
</div>
