#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUTPUT_DIR="${OUTPUT_DIR:-$SCRIPT_DIR/output}"
ROOTFS_DIR="$SCRIPT_DIR/rootfs"
KERNEL_SRC="$SCRIPT_DIR"
KERNEL_CONFIG_FRAG="$SCRIPT_DIR/kernel/configs/custom-os.config"

ARCH="${ARCH:-x86_64}"
CROSS_COMPILE="${CROSS_COMPILE:-}"

BUSYBOX_VERSION="1.35.0"
CONTAINERD_VERSION="1.7.13"
RUNC_VERSION="1.1.12"
NERDCTL_VERSION="1.7.5"
DROPBEAR_VERSION="2024.85"
COSIGN_VERSION="2.2.4"

BUSYBOX_URL="https://busybox.net/downloads/binaries/${BUSYBOX_VERSION}-x86_64-linux-musl/busybox"
CONTAINERD_URL="https://github.com/containerd/containerd/releases/download/v${CONTAINERD_VERSION}/containerd-${CONTAINERD_VERSION}-linux-amd64.tar.gz"
RUNC_URL="https://github.com/opencontainers/runc/releases/download/v${RUNC_VERSION}/runc.amd64"
NERDCTL_URL="https://github.com/containerd/nerdctl/releases/download/v${NERDCTL_VERSION}/nerdctl-${NERDCTL_VERSION}-linux-amd64.tar.gz"
DROPBEAR_URL="https://matt.ucc.asn.au/dropbear/releases/dropbear-${DROPBEAR_VERSION}.tar.bz2"

mkdir -p "$OUTPUT_DIR"

RED='\033[1;31m'; GREEN='\033[1;32m'; YELLOW='\033[1;33m'; BLUE='\033[1;34m'; NC='\033[0m'
info()  { echo -e "${BLUE}[INFO]${NC} $*" >&2; }
ok()    { echo -e "${GREEN}[OK]${NC}   $*" >&2; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*" >&2; }
err()   { echo -e "${RED}[ERROR]${NC} $*" >&2; exit 1; }

CLEAN=0
for arg in "$@"; do
    case "$arg" in
        --clean|-c) CLEAN=1 ;;
        --help|-h)
            echo "Usage: $0 [--clean] [--help]"
            echo ""
            echo "Build the Veilbox kernel and images."
            echo ""
            echo "Options:"
            echo "  --clean, -c  Remove all build artifacts and rebuild from scratch"
            echo "  --help, -h   Show this help message"
            exit 0
            ;;
    esac
done

require_tool() {
    if ! command -v "$1" &>/dev/null; then
        err "'$1' not found. Install the '$2' package for your distro."
    fi
}

require_file() {
    if [ ! -f "$1" ]; then
        err "Required file not found: $1"
    fi
}

install_deps() {
    if [[ -f /etc/os-release ]]; then
        . /etc/os-release
    fi

    case "${ID,,}" in
        fedora)
            info "Detected Fedora — checking packages..."
            if sudo -n true 2>/dev/null; then
                sudo dnf install -y \
                    squashfs-tools e2fsprogs wget tar golang \
                    qemu-system-x86 qemu-img \
                    gcc make flex bison openssl-devel \
                    elfutils-libelf-devel ncurses-devel bc rsync \
                    xz bzip2 grub2-tools grub2-pc-modules \
                    dwarves apparmor-utils iptables-legacy nftables 2>&1 | tail -3
                ok "Fedora packages installed"
            else
                warn "Sudo requires a password — skipping automated package install."
            fi
            ;;
        ubuntu|debian)
            info "Detected Debian-based distro — checking packages..."
            if sudo -n true 2>/dev/null; then
                sudo apt-get update -qq
                sudo apt-get install -y -qq \
                    squashfs-tools e2fsprogs wget golang-go \
                    qemu-system-x86 qemu-utils \
                    gcc make flex bison libssl-dev \
                    libelf-dev libncurses-dev bc rsync \
                    xz-utils bzip2 grub-pc-bin xxd \
                    apparmor-utils iptables nftables 2>&1 | tail -3
                ok "Debian packages installed"
            else
                warn "Sudo requires a password — skipping automated package install."
            fi
            ;;
        arch)
            info "Detected Arch Linux — checking packages..."
            if sudo -n true 2>/dev/null; then
                sudo pacman -Sy --noconfirm \
                    squashfs-tools e2fsprogs wget go \
                    qemu-system-x86 qemu-img \
                    gcc make flex bison openssl \
                    bc rsync xz bzip2 grub \
                    apparmor iptables nftables 2>&1 | tail -3
                ok "Arch packages installed"
            else
                warn "Sudo requires a password — skipping automated package install."
            fi
            ;;
        *)
            warn "Unknown distro '${ID:-unknown}'. Install build dependencies manually."
            ;;
    esac
}

download_binary() {
    local dest="$1" url="$2" sysbin="$3" name="$4"

    mkdir -p "$(dirname "$dest")"

    if [ -f "$dest" ]; then
        info "$name already exists at $dest, skipping."
        return 0
    fi

    info "Downloading $name..."
    if wget -q --timeout=30 "$url" -O "$dest" 2>/dev/null; then
        chmod +x "$dest"
        ok "$name downloaded to $dest"
        return 0
    fi

    warn "Failed to download $name from $url"
    rm -f "$dest"

    if [ -n "$sysbin" ] && [ -f "$sysbin" ]; then
        warn "Falling back to system binary: $sysbin"
        cp "$sysbin" "$dest" && chmod +x "$dest"
        ok "$name copied from system: $dest"
        return 0
    fi

    err "Cannot obtain $name — download failed and no system fallback at $sysbin"
}

download_tarball() {
    local url="$1" dest_dir="$2" strip_components="$3" expected_bin="$4" name="$5"

    if [ -f "$dest_dir/$expected_bin" ]; then
        info "$name already exists at $dest_dir/$expected_bin, skipping."
        return 0
    fi

    info "Downloading $name tarball..."
    local tmp_tar
    tmp_tar="$(mktemp)"

    if ! wget -q --timeout=30 "$url" -O "$tmp_tar" 2>/dev/null; then
        rm -f "$tmp_tar"
        err "Failed to download $name from $url"
    fi

    mkdir -p "$dest_dir"
    tar xf "$tmp_tar" -C "$dest_dir" --strip-components="$strip_components"
    rm -f "$tmp_tar"

    if [ -f "$dest_dir/$expected_bin" ]; then
        chmod +x "$dest_dir/$expected_bin"
        ok "$name extracted to $dest_dir/$expected_bin"
    else
        err "$name tarball extracted but expected binary $dest_dir/$expected_bin not found"
    fi
}

gen_initramfs_list() {
    local list_file="$SCRIPT_DIR/initramfs.list"
    local tmp_list
    tmp_list="$(mktemp)"

    info "Generating initramfs file list..."

    rm -f "$list_file"

    # Generate file list from rootfs directory (in subshell to preserve cwd)
    (
        cd "$ROOTFS_DIR"
        find . -print0 | while IFS= read -r -d '' f; do
            local name="${f#.}"
            [ -z "$name" ] && name="/"
            if [ -L "$f" ]; then
                echo "slink ${name} $(readlink "$f") 777 0 0"
            elif [ -f "$f" ]; then
                local mode
                mode=$(stat -c "0%a" "$f" 2>/dev/null)
                echo "file ${name} ${ROOTFS_DIR}${name} ${mode} 0 0"
            elif [ -d "$f" ]; then
                local mode
                mode=$(stat -c "0%a" "$f" 2>/dev/null)
                echo "dir ${name} ${mode} 0 0"
            fi
        done
    ) > "$tmp_list"

    # Add device nodes
    cat >> "$tmp_list" << 'DEVLIST'
nod /dev/console 622 0 0 c 5 1
nod /dev/ttyS0 600 0 0 c 4 64
DEVLIST

    # Sort for reproducibility
    sort -u "$tmp_list" > "$list_file"
    rm -f "$tmp_list"

    echo "$list_file"
}

configure_kernel() {
    info "Configuring Linux kernel..."

    cd "$KERNEL_SRC"

    require_file "$KERNEL_CONFIG_FRAG"

    if [ -f .config ]; then
        local initramfs_newer=1
        if [ ! -f "$SCRIPT_DIR/initramfs.list" ] || [ ".config" -nt "$SCRIPT_DIR/initramfs.list" ]; then
            initramfs_newer=0
        fi
        if [ ".config" -nt "$KERNEL_CONFIG_FRAG" ] && [ "$initramfs_newer" -eq 0 ]; then
            info "Using existing .config (use --clean to force reconfigure)"
            make olddefconfig
            return
        fi
    fi

    make defconfig
    scripts/kconfig/merge_config.sh -O "$KERNEL_SRC" -m .config "$KERNEL_CONFIG_FRAG"
    make olddefconfig

    ok "Kernel configured with base options"
}

strip_kernel_config() {
    info "Stripping unnecessary kernel drivers..."

    cd "$KERNEL_SRC"

    local script="$KERNEL_SRC/scripts/config"
    [ -x "$script" ] || err "scripts/config not found"

    "$script" --disable SOUND
    "$script" --disable SND
    "$script" --disable DRM
    "$script" --disable USB_SUPPORT
    "$script" --disable WLAN
    "$script" --disable BT
    "$script" --disable MEDIA_SUPPORT
    "$script" --disable VIDEO
    "$script" --disable HID
    "$script" --disable INPUT
    "$script" --disable I2C
    "$script" --disable SPI
    "$script" --disable WATCHDOG
    "$script" --disable ATA
    "$script" --disable SCSI
    "$script" --disable BLK_DEV_IO_TRACE
    "$script" --disable EDAC

    "$script" --refresh

    ok "Kernel size reduced"
}

set_initramfs_source() {
    info "Generating initramfs file list with device nodes..."

    cd "$KERNEL_SRC"

    local list_file
    list_file="$(gen_initramfs_list)"

    local script="$KERNEL_SRC/scripts/config"
    "$script" --set-str CONFIG_INITRAMFS_SOURCE "$list_file"
    make olddefconfig

    ok "Initramfs source set: $list_file"
}

build_kernel() {
    info "Building Linux kernel (this will take a while)..."
    cd "$KERNEL_SRC"
    # Limit parallel jobs to avoid OOM: use min(nproc, mem_gb)
    local jobs mem_gb
    jobs=$(nproc 2>/dev/null || echo 4)
    mem_gb=$(awk '/MemTotal/ {printf "%d", $2/1024/1024}' /proc/meminfo 2>/dev/null || echo 2)
    [ "$mem_gb" -lt 2 ] && mem_gb=2
    if [ "$jobs" -gt "$mem_gb" ]; then
        jobs=$(( (mem_gb * 3 + 3) / 4 ))  # ~0.75 * mem_gb GB per job
        [ "$jobs" -lt 1 ] && jobs=1
    fi
    make -j"$jobs" CROSS_COMPILE="$CROSS_COMPILE"
    cp arch/x86/bzImage "$OUTPUT_DIR/vmlinuz" 2>/dev/null || cp arch/x86/boot/bzImage "$OUTPUT_DIR/vmlinuz"
    ok "Kernel built: $OUTPUT_DIR/vmlinuz (${jobs} parallel jobs)"

    # Build bpftool from kernel tree for eBPF tracing
    if [ ! -f "$ROOTFS_DIR/usr/bin/bpftool" ]; then
        info "Building bpftool for eBPF tracing..."
        make -j"$jobs" -C tools/bpf/bpftool CROSS_COMPILE="$CROSS_COMPILE" >/dev/null 2>&1 || true
        local bpftool_src="tools/bpf/bpftool/bpftool"
        if [ -f "$bpftool_src" ]; then
            cp "$bpftool_src" "$ROOTFS_DIR/usr/bin/bpftool"
            ok "bpftool built and installed"
        else
            warn "bpftool build failed, eBPF tracing unavailable"
        fi
    else
        info "bpftool already exists, skipping."
    fi
}

setup_rootfs_dirs() {
    mkdir -p "$ROOTFS_DIR"/{bin,sbin,usr/bin,usr/sbin,etc/init.d,etc/dropbear,etc/containerd,dev,proc,sys,tmp,root,mnt/state,var/run,var/log,lib}
    mkdir -p "$ROOTFS_DIR/usr/share/udhcpc"
}

setup_rootfs_config() {
    mkdir -p "$ROOTFS_DIR/etc/init.d" "$ROOTFS_DIR/etc/containerd" "$ROOTFS_DIR/etc/dropbear"

    echo "veilbox" > "$ROOTFS_DIR/etc/hostname"

    echo 'root:x:0:0:root:/root:/bin/sh' > "$ROOTFS_DIR/etc/passwd"

    echo 'root:$5$2rUEXw9y7bh3/HRR$IM2VpUIeZXRc9.Fqpnmkiwo8Hg/aR/KE.GV42xZGLB/:20000:0:99999:7:::' > "$ROOTFS_DIR/etc/shadow"
    chmod 600 "$ROOTFS_DIR/etc/shadow"

    # /etc/inittab — uses autologin wrapper so veilbox.autologin kernel param
    # enables auto-login for CI/--check mode; normal boot shows login prompt.
    cat > "$ROOTFS_DIR/etc/inittab" << 'EOF'
::sysinit:/etc/init.d/rcS
::restart:/sbin/init
::shutdown:/sbin/umount -a -r

tty1::respawn:/sbin/autologin tty1 115200 vt100
ttyS0::respawn:/sbin/autologin ttyS0 115200 vt100
EOF

    # /etc/init.d/rcS
    cat > "$ROOTFS_DIR/etc/init.d/rcS" << 'EOF'
#!/bin/sh
mount -t proc none /proc
mount -t sysfs none /sys
mount -t devtmpfs none /dev

mkdir -p /dev/pts /dev/shm
mount -t devpts none /dev/pts
mount -t tmpfs none /dev/shm

hostname -F /etc/hostname

# Cgroup v2 (needed for containerd/runc resource limits)
mkdir -p /sys/fs/cgroup
mount -t cgroup2 none /sys/fs/cgroup 2>/dev/null || true

# Network initialization via net-init (handles bonding, multi-if, DHCP/static)
/sbin/net-init

# State disk: LUKS-encrypted (--keep-state) or plain VEILBOX partition
mkdir -p /mnt/state
if [ -b /dev/vdb ]; then
    # Second virtio disk (keep-state mode)
    if /sbin/cryptsetup isLuks /dev/vdb 2>/dev/null; then
        if /sbin/cryptsetup luksOpen --key-file=/etc/state.key /dev/vdb state 2>/dev/null; then
            # Check if the mapper device has a filesystem (first-boot after LUKS format)
            if ! blkid /dev/mapper/state >/dev/null 2>&1; then
                mkfs.ext4 -q /dev/mapper/state -L state
            fi
            mount /dev/mapper/state /mnt/state
        else
            # Fallback: interactive prompt
            /sbin/cryptsetup luksOpen /dev/vdb state </dev/console >/dev/console 2>&1
            mount /dev/mapper/state /mnt/state 2>/dev/null || true
        fi
    else
        mount /dev/vdb /mnt/state 2>/dev/null || mount -t tmpfs none /mnt/state
    fi
else
    mount LABEL=VEILBOX /mnt/state 2>/dev/null || mount -t tmpfs none /mnt/state
fi
mount -o remount,noexec,nodev,nosuid /mnt/state 2>/dev/null || true
mkdir -p /mnt/state/containerd /mnt/state/log /mnt/state/volumes

# Kernel networking tweaks for containers
sysctl -w net.ipv4.ip_forward=1 >/dev/null 2>&1 || true
# Increase ASLR entropy for mmap base
echo 32 > /proc/sys/vm/mmap_rnd_bits 2>/dev/null || true
modprobe br_netfilter 2>/dev/null || true
sysctl -w net.bridge.bridge-nf-call-iptables=1 >/dev/null 2>&1 || true
sysctl -w net.bridge.bridge-nf-call-ip6tables=1 >/dev/null 2>&1 || true

# Kernel hardening sysctls
sysctl -w kernel.kptr_restrict=2 >/dev/null 2>&1 || true
sysctl -w kernel.dmesg_restrict=1 >/dev/null 2>&1 || true
sysctl -w net.ipv4.conf.all.rp_filter=1 >/dev/null 2>&1 || true
sysctl -w net.ipv4.conf.all.accept_source_route=0 >/dev/null 2>&1 || true
sysctl -w net.ipv4.tcp_syncookies=1 >/dev/null 2>&1 || true

# Security subsystems
mount -t securityfs none /sys/kernel/security 2>/dev/null || true
apparmor_parser -r /etc/apparmor.d/ 2>/dev/null || true

# Host firewall: drop incoming, allow established + SSH
/sbin/iptables -P INPUT DROP 2>/dev/null || true
/sbin/iptables -P FORWARD DROP 2>/dev/null || true
/sbin/iptables -P OUTPUT ACCEPT 2>/dev/null || true
/sbin/iptables -A INPUT -i lo -j ACCEPT 2>/dev/null || true
/sbin/iptables -A INPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT 2>/dev/null || true
/sbin/iptables -A INPUT -p tcp --dport 22 -j ACCEPT 2>/dev/null || true

# Audit rules for container security events
auditctl -e 1 2>/dev/null || true
auditctl -a exit,always -F arch=b64 -S execve -k container-exec 2>/dev/null || true
auditctl -a exit,always -F arch=b64 -S clone,clone3 -k container-clone 2>/dev/null || true

# NAT for external traffic from containers
DEFAULT_IF=$(ip route show default 2>/dev/null | awk '{print $5}')
[ -z "$DEFAULT_IF" ] && DEFAULT_IF="eth0"
/sbin/iptables -t nat -A POSTROUTING -o "$DEFAULT_IF" -j MASQUERADE 2>/dev/null || true

# syslogd with 64KB circular buffer to prevent log growth on state disk
/sbin/syslogd -s 65536 -n &

mkdir -p /var/run /var/log /mnt/state/log

/usr/bin/containerd --config /etc/containerd/config.toml >/var/log/containerd.log 2>&1 &
/usr/sbin/dropbear -R -p 22

# Start health endpoint (Unix socket HTTP health check)
/usr/bin/health.sh /var/run/health.sock &

# Start QEMU guest agent (for host-guest communication via virtio-serial)
if [ -c /dev/virtio-ports/org.qemu.guest_agent.0 ]; then
    /usr/bin/qemu-ga -d -p /dev/virtio-ports/org.qemu.guest_agent.0 \
        >/var/log/qemu-ga.log 2>&1 &
fi

# Start minimal eBPF execve tracer (logs to syslog via logger)
/usr/bin/trace-exec.sh start
EOF
    chmod 755 "$ROOTFS_DIR/etc/init.d/rcS"

    cat > "$ROOTFS_DIR/etc/containerd/config.toml" << 'EOF'
root = "/mnt/state/containerd"
state = "/run/containerd"
log_level = "warn"

disabled_plugins = ["cri"]

[grpc]
  address = "/run/containerd/containerd.sock"

[metrics]
  address = ""
EOF

    cat > "$ROOTFS_DIR/usr/share/udhcpc/default.script" << 'SCRIPT'
#!/bin/sh
case "$1" in
    deconfig)
        ip addr flush dev "$interface" 2>/dev/null || true
        ip route del default 2>/dev/null || true
        ;;
    bound|renew)
        ip addr add "$ip/${subnet:-24}" dev "$interface" 2>/dev/null || true
        ip route del default 2>/dev/null || true
        [ -n "$router" ] && ip route add default via "$router" dev "$interface" 2>/dev/null || true
        # Write DNS servers from DHCP lease
        if [ -n "$dns" ]; then
            : > /etc/resolv.conf
            for ns in $dns; do
                echo "nameserver $ns" >> /etc/resolv.conf
            done
        fi
        ;;
esac
exit 0
SCRIPT
    chmod 755 "$ROOTFS_DIR/usr/share/udhcpc/default.script"

    mkdir -p "$ROOTFS_DIR/etc/network"

    # /etc/network/config — sourced by net-init; default single-interface DHCP
    cat > "$ROOTFS_DIR/etc/network/config" << 'NETCONF'
# Network configuration for net-init
# This file is sourced by /sbin/net-init at boot.
#
# Interface modes: dhcp, static, manual
# Set IFACES to a space-separated list of interface names to configure.

# --- Default: single interface, DHCP ---
IFACES="eth0"
eth0_MODE="dhcp"

# --- Static IP example ---
# IFACES="eth0"
# eth0_MODE="static"
# eth0_IP="192.168.1.100/24"
# eth0_GW="192.168.1.1"

# --- Multiple interfaces ---
# IFACES="eth0 eth1"
# eth0_MODE="dhcp"
# eth1_MODE="static"
# eth1_IP="192.168.100.10/24"
# eth1_GW="192.168.100.1"

# --- Bonded interfaces ---
# IFACES="bond0"
# bond0_MODE="dhcp"
# bond0_BOND_SLAVES="eth0 eth1"
# bond0_BOND_OPTS="mode=802.3ad miimon=100 lacp_rate=fast"

# --- Bond with static IP ---
# IFACES="bond0"
# bond0_MODE="static"
# bond0_IP="192.168.1.100/24"
# bond0_GW="192.168.1.1"
# bond0_BOND_SLAVES="eth0 eth1"
# bond0_BOND_OPTS="mode=active-backup miimon=100 primary=eth0"

# --- Policy routing rules ---
# Format: RULES="<interface> [<interface> ...]"
# For each interface in RULES, define:
#   <iface>_TABLE="<table_id>"
#   <iface>_ROUTES="<cidr> <gateway> [<cidr> <gateway> ...]"
#
# Example — route 192.168.200.0/24 via eth1's gateway:
# RULES="eth1"
# eth1_TABLE="100"
# eth1_ROUTES="192.168.200.0/24 192.168.100.1"
NETCONF

    # /sbin/net-init — network initialization script
    cat > "$ROOTFS_DIR/sbin/net-init" << 'NETINIT'
#!/bin/sh
# net-init — network initialization script
# Parses /etc/network/config and configures bonding, multi-interface,
# static IP, DHCP, and policy routing rules.
#
# Config variables per interface (IFACE is each name in IFACES):
#   IFACES          — space-separated list of interface names
#   <iface>_MODE    — dhcp (default), static, manual
#   <iface>_IP      — CIDR notation, e.g. 192.168.1.100/24 (static mode)
#   <iface>_GW      — gateway IP (static mode)
#   <iface>_BOND_SLAVES — slave interfaces for bonding, e.g. "eth0 eth1"
#   <iface>_BOND_OPTS   — bonding options, e.g. "mode=802.3ad miimon=100"
#   RULES           — space-separated list of interfaces with policy routing
#   <iface>_TABLE   — routing table ID (policy routing)
#   <iface>_ROUTES  — space-separated triples: <cidr> via <gateway>

CONFIG="/etc/network/config"
[ -f "$CONFIG" ] && . "$CONFIG"

# Fallback: no config → single eth0 DHCP (backward compatible)
if [ -z "${IFACES:-}" ]; then
    ip link set lo up
    ip link set eth0 up 2>/dev/null || true
    /sbin/udhcpc -i eth0 -b -q >/dev/null 2>&1 &
    exit 0
fi

ip link set lo up

for IFACE in $IFACES; do
    _MODE=""; eval '_MODE=$'"${IFACE}_MODE"; _MODE="${_MODE:-dhcp}"
    _BOND_SLAVES=""; eval '_BOND_SLAVES=$'"${IFACE}_BOND_SLAVES"
    _BOND_OPTS=""; eval '_BOND_OPTS=$'"${IFACE}_BOND_OPTS"

    # If this is a bonding interface, create and configure the bond
    if [ -n "$_BOND_SLAVES" ]; then
        _BOND_OPTS="${_BOND_OPTS:-mode=active-backup miimon=100}"
        if ! ip link show "$IFACE" >/dev/null 2>&1; then
            ip link add "$IFACE" type bond 2>/dev/null || true
        fi
        if [ -d "/sys/class/net/$IFACE/bonding" ]; then
            # shellcheck disable=SC2086
            for _opt in $_BOND_OPTS; do
                _key="${_opt%%=*}"
                _val="${_opt#*=}"
                [ -n "$_key" ] && printf '%s' "$_val" > "/sys/class/net/$IFACE/bonding/$_key" 2>/dev/null || true
            done
        fi
        for _slave in $_BOND_SLAVES; do
            ip link set "$_slave" down 2>/dev/null || true
            ip link set "$_slave" master "$IFACE" 2>/dev/null || true
        done
    fi

    ip link set "$IFACE" up 2>/dev/null || true

    case "$_MODE" in
        static)
            _IP=""; eval '_IP=$'"${IFACE}_IP"
            _GW=""; eval '_GW=$'"${IFACE}_GW"
            [ -n "$_IP" ] && ip addr add "$_IP" dev "$IFACE"
            [ -n "$_GW" ] && ip route add default via "$_GW" dev "$IFACE" 2>/dev/null || true
            ;;
        manual)
            ;;
        dhcp|*)
            /sbin/udhcpc -i "$IFACE" -b -q >/dev/null 2>&1 &
            ;;
    esac
done

# Policy routing — install additional rules and table entries
_RULES=""; eval '_RULES=$RULES'
if [ -n "$_RULES" ]; then
    for _RULE_IF in $_RULES; do
        _TABLE=""; eval '_TABLE=$'"${_RULE_IF}_TABLE"; _TABLE="${_TABLE:-100}"
        _ROUTES=""; eval '_ROUTES=$'"${_RULE_IF}_ROUTES"
        if [ -n "$_ROUTES" ]; then
            # Expect space-separated pairs: <cidr> <gateway> [<cidr> <gateway> ...]
            set -- $_ROUTES
            while [ $# -ge 2 ]; do
                _cidr="$1"; _gw="$2"; shift 2
                ip route add "$_cidr" via "$_gw" table "$_TABLE" 2>/dev/null || true
                ip rule add from "$_cidr" table "$_TABLE" 2>/dev/null || true
            done
        fi
    done
fi

exit 0
NETINIT
    chmod 755 "$ROOTFS_DIR/sbin/net-init"

    cat > "$ROOTFS_DIR/etc/issue" << 'EOF'
Veilbox v1.0
EOF

    # /sbin/autologin (auto-login wrapper for CI mode)
    cat > "$ROOTFS_DIR/sbin/autologin" << 'AUTOLOGIN'
#!/bin/sh
TTY="${1:-ttyS0}"
BAUD="${2:-115200}"
TERM="${3:-vt100}"

if grep -qw veilbox.autologin /proc/cmdline 2>/dev/null; then
    exec /sbin/login -f root
else
    exec /sbin/getty -L "$TTY" "$BAUD" "$TERM"
fi
AUTOLOGIN
    chmod 755 "$ROOTFS_DIR/sbin/autologin"

    # /root/.profile (colored prompt, shell builtins only)
    cat > "$ROOTFS_DIR/root/.profile" << 'PROFILE'
export PS1='\[\e[1;31m\]root@veilbox\[\e[0m\]:\[\e[1;34m\]\w\[\e[0m\]\$ '
IP=$(ip addr show eth0 2>/dev/null | grep 'inet ' | head -1)
IP="${IP#*inet }"
IP="${IP%%/*}"
[ -n "$IP" ] && echo "  IP: $IP"
echo "  Default container limits: 1 CPU, 512MB RAM (override with --cpus / --memory)"
PROFILE

    # /sbin/shutdown (BusyBox v1.35.0 lacks shutdown applet)
    cat > "$ROOTFS_DIR/sbin/shutdown" << 'SHUTDOWN'
#!/bin/sh
case "$*" in
  *-r*) exec /sbin/reboot "$@" ;;
  *-h*|*-P*|now|*) exec /sbin/poweroff "$@" ;;
esac
SHUTDOWN
    chmod 755 "$ROOTFS_DIR/sbin/shutdown"

    # Subordinate UID/GID ranges for rootless containers
    echo 'root:100000:65536' > "$ROOTFS_DIR/etc/subuid"
    echo 'root:100000:65536' > "$ROOTFS_DIR/etc/subgid"

    # Audit rules for container security events
    mkdir -p "$ROOTFS_DIR/etc/audit/rules.d"
    cat > "$ROOTFS_DIR/etc/audit/rules.d/container.rules" << 'AUDIT'
-w /usr/bin/containerd -p wa -k containerd
-w /usr/sbin/dropbear -p wa -k dropbear
-a exit,always -F arch=b64 -S execve -k process-exec
-a exit,always -F arch=b32 -S execve -k process-exec
AUDIT

    # LUKS state disk keyfile (random 32-byte key, root-only access)
    rm -f "$ROOTFS_DIR/etc/state.key"
    dd if=/dev/urandom of="$ROOTFS_DIR/etc/state.key" bs=32 count=1 2>/dev/null
    chmod 400 "$ROOTFS_DIR/etc/state.key"

    # Nerdctl config with user namespace remapping defaults
    mkdir -p "$ROOTFS_DIR/etc/nerdctl"
    cat > "$ROOTFS_DIR/etc/nerdctl/nerdctl.toml" << 'NERDCTL'
# Default nerdctl settings
namespace = "default"
cgroup_manager = "cgroupfs"
host_gateway_ip = "10.0.2.2"
NERDCTL

    ok "Rootfs config files created"
}

populate_busybox() {
    download_binary "$ROOTFS_DIR/bin/busybox" "$BUSYBOX_URL" "/usr/bin/busybox" "BusyBox"

    for applet in init mount umount reboot poweroff modprobe getty ip hostname syslogd kill mkdir ln cat echo ls ps sh sleep head grep pidof udhcpc ifconfig nc netstat df tail wc uname login passwd wget rm chmod route; do
        ln -sf /bin/busybox "$ROOTFS_DIR/sbin/$applet"
    done
    ln -sf /bin/busybox "$ROOTFS_DIR/bin/sh"
    ln -sf /bin/busybox "$ROOTFS_DIR/bin/login"
    ln -sf /sbin/init "$ROOTFS_DIR/init"
    ok "BusyBox symlinks created"
}

setup_wireguard_tools() {
    local wg_bin="$ROOTFS_DIR/usr/bin/wg"
    if [ -f "$wg_bin" ]; then
        info "wg already exists, skipping."
        return
    fi
    info "Building wg (WireGuard tools) from source..."
    if wget -q --timeout=30 \
        "https://git.zx2c4.com/wireguard-tools/snapshot/wireguard-tools-1.0.20210914.tar.xz" \
        -O /tmp/wg-tools.tar.xz 2>/dev/null; then
        local wg_src="/tmp/wg-build"
        rm -rf "$wg_src"
        mkdir -p "$wg_src"
        tar xf /tmp/wg-tools.tar.xz -C "$wg_src" --strip-components=1 2>/dev/null
        if [ -d "$wg_src/src" ]; then
            gcc -O2 -s -std=gnu99 -D_GNU_SOURCE \
                -DRUNSTATEDIR='"/var/run"' \
                -idirafter "$wg_src/src/uapi/linux" \
                "$wg_src/src/"*.c -o /tmp/wg 2>/dev/null && {
                cp /tmp/wg "$wg_bin"
                chmod 755 "$wg_bin"
                ok "wg binary built and installed"
            } || warn "Failed to compile wg"
        else
            warn "wireguard-tools source extraction failed (src/ not found in $wg_src)"
        fi
        rm -f /tmp/wg-tools.tar.xz
        rm -rf "$wg_src"
    else
        warn "Failed to download wireguard-tools; wg will not be available"
    fi
}

populate_qemu_ga() {
    local ga_bin="$ROOTFS_DIR/usr/bin/qemu-ga"
    if [ -f "$ga_bin" ]; then
        info "qemu-ga already exists, skipping."
        return
    fi
    if [ -f /usr/bin/qemu-ga ]; then
        mkdir -p "$ROOTFS_DIR/usr/bin"
        cp /usr/bin/qemu-ga "$ga_bin"
        chmod 755 "$ga_bin"
        ok "qemu-ga copied from host"
    else
        warn "qemu-ga not found on host. Install qemu-guest-agent."
    fi
}

setup_cryptsetup() {
    local cs_bin="$ROOTFS_DIR/sbin/cryptsetup"
    if [ -f "$cs_bin" ]; then
        info "cryptsetup already exists, skipping."
        return
    fi
    if [ -f /usr/sbin/cryptsetup ]; then
        mkdir -p "$ROOTFS_DIR/sbin"
        cp /usr/sbin/cryptsetup "$cs_bin"
        chmod 755 "$cs_bin"
        ok "cryptsetup copied from host"
    else
        warn "cryptsetup not found on host. LUKS state encryption will not work."
    fi
}

populate_containerd() {
    local dir="$ROOTFS_DIR/usr/bin"
    # Clean up stale top-level files from previous tarball extractions
    rm -f "$ROOTFS_DIR/usr/containerd" "$ROOTFS_DIR/usr/ctr" \
          "$ROOTFS_DIR/usr/containerd-stress" \
          "$ROOTFS_DIR/usr/containerd-shim" \
          "$ROOTFS_DIR/usr/containerd-shim-runc-v1" \
          "$ROOTFS_DIR/usr/containerd-shim-runc-v2"
    download_tarball "$CONTAINERD_URL" "$dir" 1 "containerd" "containerd"
    local shims
    shims=$(ls "$dir/containerd-shim"* 2>/dev/null || true)
    for s in $shims; do chmod +x "$s" 2>/dev/null || true; done
}

populate_runc() {
    download_binary "$ROOTFS_DIR/usr/bin/runc" "$RUNC_URL" "/usr/bin/runc" "runc"
}

populate_nerdctl() {
    # Always redownload nerdctl — the file might be our wrapper script
    rm -f "$ROOTFS_DIR/usr/bin/nerdctl" "$ROOTFS_DIR/usr/bin/nerdctl.real"
    download_tarball "$NERDCTL_URL" "$ROOTFS_DIR/usr/bin" 0 "nerdctl" "nerdctl"
    chmod +x "$ROOTFS_DIR/usr/bin/nerdctl" 2>/dev/null || true
}

populate_cni_plugins() {
    local dir="$ROOTFS_DIR/opt/cni/bin"
    local url="https://github.com/containernetworking/plugins/releases/download/v1.5.1/cni-plugins-linux-amd64-v1.5.1.tgz"
    download_tarball "$url" "$dir" 0 "bridge" "CNI plugins"
    chmod +x "$dir"/* 2>/dev/null || true

    # Default CNI bridge config
    mkdir -p "$ROOTFS_DIR/etc/cni/net.d"
    cat > "$ROOTFS_DIR/etc/cni/net.d/10-bridge.conflist" << 'CNI'
{
  "cniVersion": "0.4.0",
  "name": "default",
  "plugins": [
    {
      "type": "bridge",
      "bridge": "cni0",
      "isDefaultGateway": true,
      "ipMasq": true,
      "ipam": {
        "type": "host-local",
        "subnet": "10.88.0.0/16",
        "routes": [
          { "dst": "0.0.0.0/0" }
        ]
      }
    },
    {
      "type": "portmap",
      "capabilities": {"portMappings": true}
    },
    {
      "type": "firewall"
    }
  ]
}
CNI
    ok "Default CNI bridge config created"
}

populate_ca_certs() {
    mkdir -p "$ROOTFS_DIR/etc/ssl/certs"
    rm -f "$ROOTFS_DIR/etc/ssl/certs/ca-certificates.crt"
    if [ -f /etc/ssl/certs/ca-bundle.crt ]; then
        cp -L /etc/ssl/certs/ca-bundle.crt "$ROOTFS_DIR/etc/ssl/certs/ca-certificates.crt"
        chmod 644 "$ROOTFS_DIR/etc/ssl/certs/ca-certificates.crt"
        ok "CA certificates copied from host"
    else
        warn "No CA bundle found on host at /etc/ssl/certs/ca-bundle.crt"
    fi
}

build_dropbear() {
    local dpb_bin="$ROOTFS_DIR/usr/sbin/dropbear"
    if [ -f "$dpb_bin" ]; then
        info "Dropbear already exists, skipping."
        return
    fi

    info "Installing Dropbear from system (libraries will be copied separately)..."
    if [ -f /usr/sbin/dropbear ]; then
        mkdir -p "$ROOTFS_DIR/usr/sbin" "$ROOTFS_DIR/usr/bin"
        cp /usr/sbin/dropbear "$dpb_bin"
        chmod +x "$dpb_bin"
    else
        err "Dropbear not found at /usr/sbin/dropbear. Install the dropbear package for your distro"
    fi

    info "Generating Dropbear host keys..."
    if command -v dropbearkey &>/dev/null; then
        dropbearkey -t rsa -f "$ROOTFS_DIR/etc/dropbear/dropbear_rsa_host_key" 2>/dev/null || warn "RSA key generation failed"
        dropbearkey -t ecdsa -f "$ROOTFS_DIR/etc/dropbear/dropbear_ecdsa_host_key" 2>/dev/null || warn "ECDSA key generation failed"
        dropbearkey -t ed25519 -f "$ROOTFS_DIR/etc/dropbear/dropbear_ed25519_host_key" 2>/dev/null || warn "ED25519 key generation failed"
    else
        warn "dropbearkey not found on host; SSH host keys will be generated at first boot"
    fi

    ok "Dropbear installed from system"
}

build_ssh_keys() {
    local key_dir="$ROOTFS_DIR/root/.ssh"
    mkdir -p "$key_dir"
    if [ ! -f "$key_dir/id_ed25519" ]; then
        info "Generating SSH key pair for dropbear..."
        ssh-keygen -t ed25519 -f "$key_dir/id_ed25519" -N "" -C "root@veilbox" -q
        cp "$key_dir/id_ed25519.pub" "$key_dir/authorized_keys"
        chmod 600 "$key_dir/authorized_keys"
        chmod 600 "$key_dir/id_ed25519"
        # Also copy to /etc/dropbear for dropbear's default authorized_keys path
        cp "$key_dir/id_ed25519.pub" "$ROOTFS_DIR/etc/dropbear/authorized_keys"
        chmod 600 "$ROOTFS_DIR/etc/dropbear/authorized_keys"
        cp "$key_dir/id_ed25519" "$OUTPUT_DIR/ssh-test-key"
        chmod 600 "$OUTPUT_DIR/ssh-test-key"
        ok "SSH key pair generated"
    else
        info "SSH keys already exist, skipping."
    fi
}

copy_libraries() {
    info "Copying shared libraries for dynamic binaries..."

    rm -rf "$ROOTFS_DIR/lib64"
    mkdir -p "$ROOTFS_DIR/lib64"

    local search_bins
    search_bins=$(find "$ROOTFS_DIR" -type f -exec file {} \; | grep 'dynamically linked' | cut -d: -f1)

    while IFS= read -r bin; do
        [ -z "$bin" ] && continue

        # Capture ldd output (avoid pipeline subshell)
        local ldd_output
        ldd_output=$(ldd "$bin" 2>/dev/null || true)

        local lib_path
        while IFS= read -r line; do
            if echo "$line" | grep -q '=> /'; then
                lib_path=$(echo "$line" | sed "s/.*=> //;s/ (0x.*)//")
            elif echo "$line" | grep -q $'^\t/'; then
                lib_path=$(echo "$line" | sed "s/^\t//;s/ (0x.*)//")
            else
                continue
            fi
            cp -L "$lib_path" "$ROOTFS_DIR/lib64/" 2>/dev/null || true
        done <<< "$ldd_output"

        # Copy dynamic linker
        local interp
        interp=$(file "$bin" 2>/dev/null | sed "s/.*interpreter //;s/,.*//")
        if [ -n "$interp" ] && [ -f "$interp" ]; then
            cp -L "$interp" "$ROOTFS_DIR/lib64/" 2>/dev/null || true
        fi
    done <<< "$search_bins"

    # If ld-linux-x86-64.so.2 was copied as a file, it's correct; no symlink needed
    if [ -L "$ROOTFS_DIR/lib64/ld-linux-x86-64.so.2" ]; then
        local ld_actual
        ld_actual=$(find "$ROOTFS_DIR/lib64" -name 'ld-*' -type f ! -name 'ld-linux*' 2>/dev/null | head -1)
        if [ -n "$ld_actual" ]; then
            ln -sf "$(basename "$ld_actual")" "$ROOTFS_DIR/lib64/ld-linux-x86-64.so.2" 2>/dev/null || true
        fi
    fi

    ok "Shared libraries copied to rootfs ($(find "$ROOTFS_DIR/lib64" -type f | wc -l) files)"
}

setup_apparmor() {
    local bin="$ROOTFS_DIR/sbin/apparmor_parser"
    info "Setting up AppArmor userspace..."

    mkdir -p "$ROOTFS_DIR/sbin"

    if [ ! -f "$bin" ]; then
        # Try to copy from host first
        local src=""
        for p in /usr/sbin/apparmor_parser /sbin/apparmor_parser /usr/bin/apparmor_parser; do
            [ -f "$p" ] && { src="$p"; break; }
        done

        if [ -n "$src" ]; then
            cp "$src" "$bin"
            chmod +x "$bin"
            ok "apparmor_parser copied from host ($src)"
        else
            # Build from source (Fedora doesn't ship apparmor-utils)
            info "Building apparmor_parser from source..."
            local aa_ver="3.0.13"
            local aa_src="/tmp/apparmor-src"
            mkdir -p "$aa_src"
            if wget -q --timeout=30 \
                "https://gitlab.com/apparmor/apparmor/-/archive/v${aa_ver}/apparmor-v${aa_ver}.tar.gz" \
                -O "$aa_src/apparmor.tar.gz" 2>/dev/null; then
                tar xzf "$aa_src/apparmor.tar.gz" -C "$aa_src"
                cd "$aa_src/apparmor-v${aa_ver}"

                # Generate af_protos.h needed by libaalogparse
                echo '#include <netinet/in.h>' | \
                    gcc -E -dM - | \
                    LC_ALL=C sed -n -e "/IPPROTO_MAX/d" \
                    -e "s/^#define[ \t]\+IPPROTO_\([A-Z0-9_]\+\)\(.*\)$/AA_GEN_PROTO_ENT(\UIPPROTO_\1, \"\L\1\")/p" \
                    > libraries/libapparmor/src/af_protos.h

                # Build libapparmor.a manually (avoids autotools/libtool dependency)
                local lib_src="libraries/libapparmor/src"
                local lib_inc="libraries/libapparmor/include"
                local CFLAGS="-O2 -fPIC -D_GNU_SOURCE -D_DEFAULT_SOURCE -I$lib_inc -I$lib_src"
                for f in features.c kernel.c kernel_interface.c PMurHash.c private.c libaalogparse.c policy_cache.c; do
                    gcc -c $CFLAGS "$lib_src/$f" -o "/tmp/aa_build_${f%.c}.o"
                done
                mkdir -p "$lib_src/.libs"
                (cd /tmp && ar rcs "$OLDPWD/$lib_src/.libs/libapparmor.a" aa_build_*.o)

                # Build parser (dynamically linked — libstdc++ .a not available)
                # pod2html may not be installed; skip HTML doc generation
                make -C parser -j"$(nproc)" \
                    "AARE_LDFLAGS=-L. -L../$lib_src/.libs" \
                    "POD2HTML=true" 2>&1 | tail -3
                if [ -f "parser/apparmor_parser" ]; then
                    cp parser/apparmor_parser "$bin"
                    chmod +x "$bin"
                    ok "apparmor_parser built from source"
                else
                    warn "Failed to build apparmor_parser from source. AppArmor disabled."
                    rm -f "$bin"
                fi
                cd /
                rm -rf "$aa_src" /tmp/aa_build_*.o
            else
                warn "Failed to download AppArmor source. AppArmor disabled."
            fi
        fi
    else
        info "apparmor_parser already exists, skipping build."
    fi

    if [ ! -f "$bin" ]; then
        warn "apparmor_parser not available. AppArmor profiles will not be loaded."
        return
    fi

    local aad="$ROOTFS_DIR/etc/apparmor.d"
    mkdir -p "$aad"

    cat > "$aad/docker-default" << 'AAPROFILE'
profile docker-default flags=(attach_disconnected,mediate_deleted) {
  capability,
  network,
  mount,
  umount,
  pivot_root,

  / r,
  /** rwm,
  /bin/** rix,
  /usr/** rix,
  /lib/** rix,
  /opt/** rix,
  /sbin/** rix,

  /mnt/state/** rw,

  deny /sys/[^f]*/** w,
  deny /sys/firmware/** w,
  deny /sys/kernel/security/** w,
  deny /proc/sysrq-trigger w,
  deny /proc/kcore r,
  deny /proc/kmsg r,
  deny /proc/keys r,
}
AAPROFILE
    ok "Docker AppArmor profile created at $aad/docker-default"
}

setup_iptables() {
    local bin="$ROOTFS_DIR/sbin/iptables"

    if [ ! -f "$bin" ]; then
        info "Setting up iptables userspace..."

        mkdir -p "$ROOTFS_DIR/sbin"

        # Prefer iptables-legacy, fall back to iptables
        local src=""
        for p in /usr/sbin/iptables-legacy /usr/sbin/iptables /sbin/iptables-legacy /sbin/iptables; do
            [ -f "$p" ] && { src="$p"; break; }
        done

        if [ -n "$src" ]; then
            cp "$src" "$bin"
            chmod +x "$bin"
            ok "iptables copied from host ($src)"
        else
            warn "iptables not found on host. Container NAT may not work."
            return
        fi

        # Symlink ip6tables if available
        for p in /usr/sbin/ip6tables-legacy /sbin/ip6tables-legacy /usr/sbin/ip6tables /sbin/ip6tables; do
            if [ -f "$p" ]; then
                cp "$p" "$ROOTFS_DIR/sbin/ip6tables"
                chmod +x "$ROOTFS_DIR/sbin/ip6tables"
                break
            fi
        done
    else
        info "iptables already exists, skipping binary copy."
    fi

    # Always copy xtables plugins (libxt_*.so) loaded via dlopen by libxtables.so
    # This must run every build to ensure rootfs has the latest plugins
    local xt_dirs=("/usr/lib64/xtables" "/usr/lib/xtables")
    local xt_dir=""
    for d in "${xt_dirs[@]}"; do
        if [ -d "$d" ]; then
            xt_dir="$d"
            break
        fi
    done
    if [ -n "$xt_dir" ]; then
        # Copy to both lib and lib64 to match whatever the iptables binary expects
        local xt_targets=("$ROOTFS_DIR/usr/lib/xtables" "$ROOTFS_DIR/usr/lib64/xtables")
        local t
        for t in "${xt_targets[@]}"; do
            mkdir -p "$t"
            cp -L "$xt_dir"/libxt_*.so "$t/" 2>/dev/null || true
        done
        local count
        count=$(ls "$ROOTFS_DIR/usr/lib/xtables/"libxt_*.so 2>/dev/null | wc -l)
        info "Copied $count xtables plugins from $xt_dir"
    else
        warn "xtables plugins directory not found on host"
    fi
}

setup_audit() {
    local bin="$ROOTFS_DIR/sbin/auditctl"
    if [ -f "$bin" ]; then
        info "auditctl already exists, skipping."
        return
    fi
    local src=""
    for p in /usr/sbin/auditctl /sbin/auditctl /usr/bin/auditctl; do
        [ -f "$p" ] && { src="$p"; break; }
    done
    if [ -n "$src" ]; then
        mkdir -p "$ROOTFS_DIR/sbin"
        cp "$src" "$bin"
        chmod +x "$bin"
        ok "auditctl copied from host ($src)"
    else
        warn "auditctl not found on host. Audit logging will be limited."
    fi
}

setup_cosign() {
    local bin="$ROOTFS_DIR/usr/bin/cosign"
    if [ -f "$bin" ]; then
        info "cosign already exists, skipping."
        return
    fi
    info "Downloading cosign for image signature verification..."
    mkdir -p "$ROOTFS_DIR/usr/bin"
    local url="https://github.com/sigstore/cosign/releases/download/v${COSIGN_VERSION}/cosign-linux-amd64"
    if wget -q --timeout=30 "$url" -O "$bin" 2>/dev/null; then
        chmod +x "$bin"
        ok "cosign downloaded"
    else
        warn "Failed to download cosign. Image verification will require manual setup."
        rm -f "$bin"
    fi
}

set_rootfs_perms() {
    chmod 755 "$ROOTFS_DIR/etc/init.d/rcS" 2>/dev/null || true
    chmod 700 "$ROOTFS_DIR/root" 2>/dev/null || true
    chmod 1777 "$ROOTFS_DIR/tmp" 2>/dev/null || true
    chmod 755 "$ROOTFS_DIR/usr/bin/trace-exec.sh" 2>/dev/null || true
    chmod 755 "$ROOTFS_DIR/usr/bin/health.sh" 2>/dev/null || true
}

create_state_img() {
    local img="$OUTPUT_DIR/state.img"
    local keyfile="$ROOTFS_DIR/etc/state.key"
    info "Creating state image (128MB)..."
    rm -f "$img"
    dd if=/dev/zero of="$img" bs=1M count=128 status=progress
    if [ -f "$keyfile" ] && command -v cryptsetup &>/dev/null; then
        # Format LUKS header (works without root — only writes metadata)
        # The filesystem inside is created on first boot by rcS
        cryptsetup luksFormat --batch-mode --key-file="$keyfile" "$img" 2>/dev/null && \
            ok "LUKS-encrypted state image created: $img (filesystem created at first boot)" || \
            warn "LUKS format failed, falling back to plain ext4"
    fi
    # If the file is not a LUKS container (fallback), create plain ext4
    if ! cryptsetup isLuks "$img" 2>/dev/null; then
        mkfs.ext4 -q -F "$img" -L state
        info "Plain ext4 state image created: $img"
    fi
}

create_rootfs_squashfs() {
    local img="$OUTPUT_DIR/rootfs.squashfs"
    info "Creating SquashFS image (xz compression)..."
    rm -f "$img"
    mksquashfs "$ROOTFS_DIR" "$img" -comp xz -all-root -noappend
    local size
    size=$(du -h "$img" | cut -f1)
    ok "SquashFS image created: $img ($size)"
}

create_bootable_disk() {
    local raw="$OUTPUT_DIR/veilbox.raw"
    local img_size="${DISK_SIZE:-8}"  # GB
    local fs_img="$OUTPUT_DIR/.veilbox_fs.tmp"

    info "Creating bootable disk image (${img_size}GB)..."
    rm -f "$raw" "$fs_img"

    info "Creating ext4 filesystem..."
    dd if=/dev/zero of="$fs_img" bs=1M count=$((img_size * 1024 - 12)) status=progress
    mkfs.ext4 -F -L VEILBOX "$fs_img" > /dev/null 2>&1

    info "Populating filesystem with kernel and config..."

    debugfs -w -R "mkdir /boot" "$fs_img" 2>/dev/null || true
    debugfs -w -R "mkdir /boot/grub" "$fs_img" 2>/dev/null || true
    debugfs -w -R "mkdir /state" "$fs_img" 2>/dev/null || true

    debugfs -w -R "write $OUTPUT_DIR/vmlinuz /boot/vmlinuz" "$fs_img" 2>/dev/null || {
        rm -f "$fs_img"
        err "Failed to copy kernel into filesystem"
    }

    local grub_cfg_tmp
    grub_cfg_tmp="$(mktemp /tmp/veilbox-grubcfg-XXXXXX)"
    cat > "$grub_cfg_tmp" << 'GRUB'
set timeout=3
set default=0

serial --unit=0 --speed=115200
terminal_input serial console
terminal_output serial console

menuentry "Veilbox" {
    search --label --set=root VEILBOX
    linux /boot/vmlinuz console=tty0 console=ttyS0 quiet
}
GRUB
    debugfs -w -R "write $grub_cfg_tmp /boot/grub/grub.cfg" "$fs_img" 2>/dev/null || true
    rm -f "$grub_cfg_tmp"

    # Copy GRUB modules into the filesystem
    local grub_mod_dir="/usr/lib/grub/i386-pc"
    if [ -d "$grub_mod_dir" ]; then
        info "Copying GRUB modules..."
        for mod in ext2 part_msdos biosdisk configfile search_label serial terminal linux gzio fshelp search; do
            [ -f "$grub_mod_dir/${mod}.mod" ] && \
                debugfs -w -R "write $grub_mod_dir/${mod}.mod /boot/grub/${mod}.mod" "$fs_img" 2>/dev/null || true
        done
    fi

    info "Creating disk layout..."
    dd if=/dev/zero of="$raw" bs=1M count=$((img_size * 1024)) status=progress
    printf 'type=83, bootable\n' | sfdisk "$raw" > /dev/null 2>&1

    info "Embedding filesystem into partition..."
    dd if="$fs_img" of="$raw" bs=512 seek=2048 conv=notrunc status=progress
    rm -f "$fs_img"

    info "Installing GRUB bootloader..."
    PYTHON_RAW="$raw" GRUB_MKIMAGE="$GRUB_MKIMAGE" python3 << 'PYGRUB' || err "GRUB installation failed"
import subprocess, os, sys, tempfile

raw_path = os.environ["PYTHON_RAW"]
grub_mkimage = os.environ["GRUB_MKIMAGE"]
boot_img_path = "/usr/lib/grub/i386-pc/boot.img"
core_img_mods = ["ext2", "part_msdos", "biosdisk", "configfile",
                 "search_label", "serial", "terminal", "linux",
                 "gzio", "fshelp", "search"]

# Embedded config: search for VEILBOX by label, set prefix
with tempfile.NamedTemporaryFile(mode='w', suffix='.cfg', delete=False) as f:
    f.write('search --label --set=root VEILBOX\n')
    f.write('set prefix=($root)/boot/grub\n')
    embedded_cfg = f.name

# Generate core.img via grub-mkimage
result = subprocess.run(
    [grub_mkimage, "-c", embedded_cfg,
     "-O", "i386-pc", "-o", "/tmp/veilbox-core.img",
     "-p", "/boot/grub"] + core_img_mods,
    capture_output=True
)
os.unlink(embedded_cfg)
if result.returncode != 0:
    print("grub2-mkimage failed:", result.stderr.decode())
    sys.exit(1)

with open("/tmp/veilbox-core.img", "rb") as f:
    core = f.read()
core_sectors = (len(core) + 511) // 512
print(f"core.img: {len(core)} bytes = {core_sectors} sectors")

# Save the partition table (sfdisk wrote it at 0x1BE-0x1FD)
with open(raw_path, "rb") as f:
    raw_mbr = bytearray(f.read(512))
partition_table = raw_mbr[0x1BE:0x1FE]

# Write boot.img to LBA 0 (preserving partition table and 0x55AA)
with open(boot_img_path, "rb") as f:
    boot = f.read(512)
with open(raw_path, "r+b") as f:
    f.write(boot)
    # Restore partition table and boot signature
    f.seek(0x1BE)
    f.write(partition_table)
    f.write(b'\x55\xAA')

# Write core.img starting at LBA 1
with open(raw_path, "r+b") as f:
    f.seek(512)
    f.write(core)
    pad = core_sectors * 512 - len(core)
    if pad > 0:
        f.write(b'\x00' * pad)

os.unlink("/tmp/veilbox-core.img")
print(f"GRUB installed: boot.img at LBA 0, core.img at LBA 1-{core_sectors}")
PYGRUB

    ok "Bootable disk created: $raw ($img_size GB)"
}

convert_to_vdi() {
    local raw="$OUTPUT_DIR/veilbox.raw"
    local vdi="$OUTPUT_DIR/veilbox.vdi"

    if [ ! -f "$raw" ]; then
        err "Raw disk not found at $raw — run create_bootable_disk first"
    fi

    info "Converting raw disk to VDI..."
    qemu-img convert -f raw -O vdi "$raw" "$vdi"
    local size
    size=$(du -h "$vdi" | cut -f1)
    ok "VDI image created: $vdi ($size)"
}

if [ "$CLEAN" -eq 1 ]; then
    echo "Cleaning build artifacts..."
    rm -rf "$ROOTFS_DIR" "$OUTPUT_DIR" "$SCRIPT_DIR/initramfs.list" "$KERNEL_SRC/.config"
    echo "Clean complete."
    echo ""
fi
mkdir -p "$OUTPUT_DIR"

echo ""
echo "============================================"
echo "  Veilbox Builder"
echo "============================================"
echo ""

install_deps

require_tool make "make"
require_tool gcc "gcc"
require_tool go "golang"
require_tool wget "wget"
require_tool mksquashfs "squashfs-tools"
require_tool mkfs.ext4 "e2fsprogs"
require_tool xz "xz"
require_tool bzip2 "bzip2"
require_tool dd "coreutils"
require_tool python3 "python3"
# Detect GRUB mkimage tool
GRUB_MKIMAGE=""
if command -v grub2-mkimage &>/dev/null; then
    GRUB_MKIMAGE="grub2-mkimage"
elif command -v grub-mkimage &>/dev/null; then
    GRUB_MKIMAGE="grub-mkimage"
fi
if [ -z "$GRUB_MKIMAGE" ]; then
    err "grub2-mkimage or grub-mkimage not found. Install grub2-tools (Fedora) or grub-pc-bin (Debian) or grub (Arch)."
fi
require_tool debugfs "e2fsprogs"
require_tool sfdisk "util-linux"
require_tool qemu-img "qemu-img"

setup_rootfs_dirs
setup_rootfs_config
configure_kernel
strip_kernel_config

populate_busybox
populate_containerd
populate_runc
populate_nerdctl
# Rename real binary and create wrapper for default resource limits
if [ -f "$ROOTFS_DIR/usr/bin/nerdctl.real" ]; then
    rm -f "$ROOTFS_DIR/usr/bin/nerdctl.real"
fi
if [ -f "$ROOTFS_DIR/usr/bin/nerdctl" ]; then
    mv "$ROOTFS_DIR/usr/bin/nerdctl" "$ROOTFS_DIR/usr/bin/nerdctl.real"
fi
if [ ! -f "$ROOTFS_DIR/usr/bin/nerdctl" ]; then
    cat > "$ROOTFS_DIR/usr/bin/nerdctl" << 'WRAPPER'
#!/bin/sh
EXTRA=""; HAS_CPUS=0; HAS_MEM=0; SEEN=0
# Scan args for overrides and subcommand
for arg in "$@"; do
    case "$arg" in
        --cpus=*|--cpuset-cpus=*) HAS_CPUS=1 ;;
        --memory=*|-m) HAS_MEM=1 ;;
        run|create) SEEN=1 ;;
    esac
done
# Build extra flags to inject after subcommand
if [ "$SEEN" = 1 ]; then
    [ "$HAS_CPUS" = 0 ] && EXTRA="--cpus=1"
    [ "$HAS_MEM" = 0 ] && EXTRA="$EXTRA --memory=512m"
fi
OUT=""; INSERTED=0
for arg in "$@"; do
    if [ "$INSERTED" = 0 ] && { [ "$arg" = "run" ] || [ "$arg" = "create" ]; }; then
        OUT="$OUT $arg $EXTRA"
        INSERTED=1
    else
        OUT="$OUT $arg"
    fi
done
exec /usr/bin/nerdctl.real $OUT
WRAPPER
    chmod 755 "$ROOTFS_DIR/usr/bin/nerdctl"
    ok "nerdctl wrapper created (default: 1 CPU, 512MB RAM)"
fi
ln -sf /usr/bin/nerdctl "$ROOTFS_DIR/usr/bin/docker"
ok "docker → nerdctl symlink created"
build_dropbear
setup_apparmor
setup_iptables
setup_audit
setup_cosign
setup_wireguard_tools
populate_qemu_ga
setup_cryptsetup
build_ssh_keys

copy_libraries
set_rootfs_perms

populate_cni_plugins
populate_ca_certs
set_initramfs_source
build_kernel
create_rootfs_squashfs
create_state_img
create_bootable_disk
convert_to_vdi

echo ""
echo "============================================"
echo "  Build complete!"
echo "  Bootable disk:  $OUTPUT_DIR/veilbox.raw"
echo "  VirtualBox VDI: $OUTPUT_DIR/veilbox.vdi"
echo "  Kernel:         $OUTPUT_DIR/vmlinuz"
echo "  State disk:     $OUTPUT_DIR/state.img (LUKS-encrypted if cryptsetup available)"
echo "  Features:       WireGuard VPN, QEMU guest agent, LUKS state encryption"
echo "============================================"
echo ""
echo "To test with QEMU:"
echo "  ./test.sh"
echo ""
echo "To import into VirtualBox:"
echo "  Create a new VM, attach output/veilbox.vdi as the disk"
echo ""
echo "To write to a real disk (CAUTION: overwrites target):"
echo "  dd if=output/veilbox.raw of=/dev/sdX bs=4M status=progress"
echo ""
echo "SSH into the running VM:"
echo "  ssh -i output/ssh-test-key root@localhost -p 2222"
echo "    or with password: ssh root@localhost -p 2222 (password: veiladmin)"
echo ""
