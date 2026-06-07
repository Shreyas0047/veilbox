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
DROPBEAR_VERSION="2025.89"

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

# Parse command line arguments
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
        err "'$1' not found in PATH. Install with: dnf install $2"
    fi
}

require_file() {
    if [ ! -f "$1" ]; then
        err "Required file not found: $1"
    fi
}

# ---------- 0. Install dependencies (Fedora) ----------
install_deps() {
    if [ ! -f /etc/fedora-release ]; then
        warn "Not running Fedora — skipping package install. Ensure build dependencies are installed."
        return
    fi

    info "Detected Fedora — checking packages..."
    if sudo -n true 2>/dev/null; then
        sudo dnf install -y \
            squashfs-tools e2fsprogs wget tar golang \
            qemu-system-x86 qemu-img \
            containerd nerdctl dropbear busybox-static \
            kernel-devel gcc make flex bison openssl-devel \
            elfutils-libelf-devel ncurses-devel bc rsync \
            xz bzip2 grub2-tools grub2-pc-modules 2>&1 | tail -3
        ok "Fedora packages installed"
    else
        warn "Sudo requires a password — skipping automated package install."
        warn "If tools are missing, run: sudo dnf install -y squashfs-tools e2fsprogs wget tar golang qemu-system-x86 qemu-img containerd nerdctl dropbear busybox-static kernel-devel gcc make flex bison openssl-devel elfutils-libelf-devel ncurses-devel bc rsync xz bzip2 grub2-tools grub2-pc-modules"
    fi
}

# ---------- 1. Download a static binary with fallback ----------
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

# ---------- 2. Generate initramfs file list (with device nodes) ----------
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

# ---------- 3. Configure kernel (base options, no initramfs yet) ----------
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

# ---------- 4. Strip unnecessary kernel drivers ----------
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

    "$script" --refresh

    ok "Kernel size reduced"
}

# ---------- 5. Set initramfs source (after rootfs is populated) ----------
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

# ---------- 6. Build kernel ----------
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
}

# ---------- 7. Set up rootfs directory structure ----------
setup_rootfs_dirs() {
    mkdir -p "$ROOTFS_DIR"/{bin,sbin,usr/bin,usr/sbin,etc/init.d,etc/dropbear,etc/containerd,dev,proc,sys,tmp,root,mnt/state,var/run,var/log,lib}
    mkdir -p "$ROOTFS_DIR/usr/share/udhcpc"
}

# ---------- 8. Create rootfs config files (survive --clean) ----------
setup_rootfs_config() {
    mkdir -p "$ROOTFS_DIR/etc/init.d" "$ROOTFS_DIR/etc/containerd" "$ROOTFS_DIR/etc/dropbear"

    # /etc/hostname
    echo "veilbox" > "$ROOTFS_DIR/etc/hostname"

    # /etc/passwd (root with shell at /bin/sh)
    echo 'root:x:0:0:root:/root:/bin/sh' > "$ROOTFS_DIR/etc/passwd"

    # /etc/shadow (root with password veiladmin)
    echo 'root:$5$2rUEXw9y7bh3/HRR$IM2VpUIeZXRc9.Fqpnmkiwo8Hg/aR/KE.GV42xZGLB/:20000:0:99999:7:::' > "$ROOTFS_DIR/etc/shadow"
    chmod 600 "$ROOTFS_DIR/etc/shadow"

    # /etc/inittab — uses autologin wrapper so veilbox.autologin kernel param
    # enables auto-login for CI/--check mode; normal boot shows login prompt.
    cat > "$ROOTFS_DIR/etc/inittab" << 'EOF'
::sysinit:/etc/init.d/rcS
::restart:/sbin/init
::shutdown:/bin/umount -a -r
::shutdown:/sbin/swapoff -a

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

mkdir -p /mnt/state
mount LABEL=VEILBOX /mnt/state 2>/dev/null || mount -t tmpfs none /mnt/state
mkdir -p /mnt/state/containerd /mnt/state/log /mnt/state/volumes

ip link set lo up
ip link set eth0 up 2>/dev/null || true

echo "nameserver 10.0.2.3" > /etc/resolv.conf
/sbin/udhcpc -i eth0 -b -q &

/sbin/syslogd -n &

mkdir -p /var/run /var/log

/usr/bin/containerd --config /etc/containerd/config.toml &
/usr/sbin/dropbear -R -p 22
EOF
    chmod 755 "$ROOTFS_DIR/etc/init.d/rcS"

    # /etc/containerd/config.toml
    cat > "$ROOTFS_DIR/etc/containerd/config.toml" << 'EOF'
root = "/mnt/state/containerd"
state = "/run/containerd"

disabled_plugins = ["cri"]

[grpc]
  address = "/run/containerd/containerd.sock"

[metrics]
  address = ""
EOF

    # /usr/share/udhcpc/default.script
    cat > "$ROOTFS_DIR/usr/share/udhcpc/default.script" << 'SCRIPT'
#!/bin/sh
case "$1" in
    deconfig)
        ip addr flush dev $interface 2>/dev/null || true
        ip route del default 2>/dev/null || true
        ;;
    bound|renew)
        ip addr add $ip/${subnet:-24} dev $interface 2>/dev/null || true
        ip route del default 2>/dev/null || true
        [ -n "$router" ] && ip route add default via $router dev $interface 2>/dev/null || true
        ;;
esac
exit 0
SCRIPT
    chmod 755 "$ROOTFS_DIR/usr/share/udhcpc/default.script"

    # /etc/issue (login banner)
    cat > "$ROOTFS_DIR/etc/issue" << 'EOF'
Veilbox v1.0
EOF

    # /root/.profile (colored prompt)
    cat > "$ROOTFS_DIR/root/.profile" << 'PROFILE'
export PS1='\[\e[1;31m\]root@veilbox\[\e[0m\]:\[\e[1;34m\]\w\[\e[0m\]\$ '
IP=$(ip addr show eth0 2>/dev/null | grep 'inet ' | awk '{print $2}' | cut -d/ -f1)
[ -n "$IP" ] && echo "  IP: $IP"
PROFILE

    ok "Rootfs config files created"
}

# ---------- 9. Populate rootfs with static binaries ----------
populate_busybox() {
    download_binary "$ROOTFS_DIR/bin/busybox" "$BUSYBOX_URL" "/usr/bin/busybox" "BusyBox"

    for applet in init mount umount reboot poweroff modprobe getty ip hostname syslogd kill mkdir ln cat echo ls ps sh sleep head grep pidof udhcpc ifconfig nc netstat df tail wc uname login passwd; do
        ln -sf /bin/busybox "$ROOTFS_DIR/sbin/$applet"
    done
    ln -sf /bin/busybox "$ROOTFS_DIR/bin/sh"
    ln -sf /sbin/init "$ROOTFS_DIR/init"
    ok "BusyBox symlinks created"
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
    download_tarball "$NERDCTL_URL" "$ROOTFS_DIR/usr/bin" 0 "nerdctl" "nerdctl"
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
        err "Dropbear not found at /usr/sbin/dropbear. Install with: dnf install -y dropbear"
    fi

    # Generate host keys using a locally running dropbearkey
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
        # Export test key for host use
        cp "$key_dir/id_ed25519" "$OUTPUT_DIR/ssh-test-key"
        chmod 600 "$OUTPUT_DIR/ssh-test-key"
        ok "SSH key pair generated"
    else
        info "SSH keys already exist, skipping."
    fi
}

# ---------- 9. Copy shared libraries for dynamic binaries ----------
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

# ---------- 10. Set rootfs permissions ----------
set_rootfs_perms() {
    chmod 755 "$ROOTFS_DIR/etc/init.d/rcS" 2>/dev/null || true
    chmod 700 "$ROOTFS_DIR/root" 2>/dev/null || true
    chmod 1777 "$ROOTFS_DIR/tmp" 2>/dev/null || true
}

# ---------- 11. Create state image ----------
create_state_img() {
    local img="$OUTPUT_DIR/state.img"
    info "Creating state image (ext4, 128MB)..."
    rm -f "$img"
    dd if=/dev/zero of="$img" bs=1M count=128 status=progress
    mkfs.ext4 -q -F "$img" -L state
    ok "State image created: $img"
}

# ---------- 12. Create rootfs.squashfs ----------
create_rootfs_squashfs() {
    local img="$OUTPUT_DIR/rootfs.squashfs"
    info "Creating SquashFS image (xz compression)..."
    rm -f "$img"
    mksquashfs "$ROOTFS_DIR" "$img" -comp xz -all-root -noappend
    local size
    size=$(du -h "$img" | cut -f1)
    ok "SquashFS image created: $img ($size)"
}

# ---------- 13. Create bootable disk image with GRUB ----------
create_bootable_disk() {
    local raw="$OUTPUT_DIR/veilbox.raw"
    local img_size="${DISK_SIZE:-8}"  # GB
    local fs_img="$OUTPUT_DIR/.veilbox_fs.tmp"

    info "Creating bootable disk image (${img_size}GB)..."
    rm -f "$raw" "$fs_img"

    # Step 1: Create standalone ext4 filesystem image at OUTPUT_DIR (for disk space)
    info "Creating ext4 filesystem..."
    dd if=/dev/zero of="$fs_img" bs=1M count=$((img_size * 1024 - 12)) status=progress
    mkfs.ext4 -F -L VEILBOX "$fs_img" > /dev/null 2>&1

    # Step 2: Populate using debugfs (no root required)
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
    linux /boot/vmlinuz console=tty0 console=ttyS0
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

    # Step 3: Create partitioned disk image
    info "Creating disk layout..."
    dd if=/dev/zero of="$raw" bs=1M count=$((img_size * 1024)) status=progress
    printf 'type=83, bootable\n' | sfdisk "$raw" > /dev/null 2>&1

    # Step 4: Embed filesystem into partition 1 (starts at sector 2048)
    info "Embedding filesystem into partition..."
    dd if="$fs_img" of="$raw" bs=512 seek=2048 conv=notrunc status=progress
    rm -f "$fs_img"

    # Step 5: Install GRUB rootlessly — preserve partition table
    info "Installing GRUB bootloader..."
    PYTHON_RAW="$raw" python3 << 'PYGRUB' || err "GRUB installation failed"
import subprocess, os, sys, tempfile

raw_path = os.environ["PYTHON_RAW"]
boot_img_path = "/usr/lib/grub/i386-pc/boot.img"
core_img_mods = ["ext2", "part_msdos", "biosdisk", "configfile",
                 "search_label", "serial", "terminal", "linux",
                 "gzio", "fshelp", "search"]

# Embedded config: search for VEILBOX by label, set prefix
with tempfile.NamedTemporaryFile(mode='w', suffix='.cfg', delete=False) as f:
    f.write('search --label --set=root VEILBOX\n')
    f.write('set prefix=($root)/boot/grub\n')
    embedded_cfg = f.name

# Generate core.img via grub2-mkimage (blocklist already correct)
result = subprocess.run(
    ["grub2-mkimage", "-c", embedded_cfg,
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

# ---------- 14. Convert raw disk to VDI ----------
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

# ---------- Main ----------
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
require_tool grub2-mkimage "grub2-tools"
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
build_dropbear
build_ssh_keys

copy_libraries
set_rootfs_perms

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
echo "  State disk:     $OUTPUT_DIR/state.img"
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
