#!/usr/bin/env bash
# Veilbox — QEMU / VirtualBox test runner
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUTPUT_DIR="${OUTPUT_DIR:-$SCRIPT_DIR/output}"
QEMU_BINARY="${QEMU_BINARY:-qemu-system-x86_64}"
MEM="${MEM:-2G}"
SMP="${SMP:-2}"
SSH_PORT="${SSH_PORT:-2222}"
TIMEOUT="${TIMEOUT:-0}"
LOG_FILE="${LOG_FILE:-}"
CHECK="${CHECK:-}"
AUTOLOGIN="${AUTOLOGIN:-}"
KEEP_STATE="${KEEP_STATE:-}"
VBOX="${VBOX:-}"
VBOX_CREATE="${VBOX_CREATE:-}"
VM_NAME="${VM_NAME:-veilbox}"

KERNEL="$OUTPUT_DIR/vmlinuz"
DISK_IMG="$OUTPUT_DIR/veilbox.raw"
VDI_IMG="$OUTPUT_DIR/veilbox.vdi"
STATE_PERSIST="$OUTPUT_DIR/state-persist.img"
QEMU_IMG="${QEMU_IMG:-qemu-img}"
BIOS="${BIOS:-}"
RED='\033[1;31m'; GREEN='\033[1;32m'; YELLOW='\033[1;33m'; BLUE='\033[1;34m'; NC='\033[0m'

usage() {
    echo "Usage: $0 [options]"
    echo ""
    echo "QEMU options:"
    echo "  --bios          Boot via SeaBIOS/GRUB (disk image) instead of direct kernel"
    echo "  --autologin     Auto-login as root on serial console (direct kernel only)"
    echo "  --keep-state    Preserve state disk across reboots (external state-persist.img)"
    echo "  --timeout SECS  Automatically exit after SECS seconds"
    echo "  --output FILE   Log serial console output to FILE"
    echo "  --check         Exit with status 0 if VM boots to login prompt"
    echo ""
    echo "VirtualBox options:"
    echo "  --vbox          Boot in VirtualBox (creates VM if needed)"
    echo "  --vbox-create   Register VM in VirtualBox without starting it"
    echo "  --vm-name NAME  VirtualBox VM name (default: veilbox)"
    echo ""
    echo "Common:"
    echo "  --help, -h      Show this help message"
    echo ""
    echo "Environment variables: MEM, SMP, SSH_PORT, QEMU_BINARY, VM_NAME"
    exit 0
}

for arg in "$@"; do
    case "$arg" in
        --bios)         BIOS=1 ;;
        --autologin)    AUTOLOGIN=1 ;;
        --keep-state)   KEEP_STATE=1 ;;
        --timeout=*)    TIMEOUT="${arg#*=}" ;;
        --output=*)     LOG_FILE="${arg#*=}" ;;
        --check)        CHECK=1 ;;
        --vbox)         VBOX=1 ;;
        --vbox-create)  VBOX_CREATE=1 ;;
        --vm-name=*)    VM_NAME="${arg#*=}" ;;
        --help|-h)      usage ;;
    esac
done

# ---------- VirtualBox mode ----------
if [ -n "$VBOX" ] || [ -n "$VBOX_CREATE" ]; then
    VBOX_MANAGE="${VBOX_MANAGE:-$(command -v VBoxManage 2>/dev/null || true)}"
    if [ -z "$VBOX_MANAGE" ]; then
        echo -e "${RED}ERROR${NC}: VBoxManage not found. Install VirtualBox or set VBOX_MANAGE."
        exit 1
    fi
    if [ ! -f "$VDI_IMG" ]; then
        echo -e "${RED}ERROR${NC}: VDI not found at $VDI_IMG"
        echo "Run ./build.sh first."
        exit 1
    fi

    if "$VBOX_MANAGE" list vms | grep -q "\"$VM_NAME\"" 2>/dev/null; then
        echo -e "${BLUE}[INFO]${NC} VM '$VM_NAME' already registered."
    else
        echo -e "${BLUE}[INFO]${NC} Creating VM '$VM_NAME'..."
        "$VBOX_MANAGE" createvm --name "$VM_NAME" --ostype "Linux_64" --register
        "$VBOX_MANAGE" modifyvm "$VM_NAME" --memory "${MEM%G}" --cpus "$SMP" --ioapic on
        "$VBOX_MANAGE" modifyvm "$VM_NAME" --nic1 nat --natpf1 "ssh,tcp,,${SSH_PORT},,22"
        "$VBOX_MANAGE" modifyvm "$VM_NAME" --graphicscontroller vmsvga
        "$VBOX_MANAGE" modifyvm "$VM_NAME" --boot1 disk

        SATA_CTL=$(printf '%s' "SATA Controller")
        "$VBOX_MANAGE" storagectl "$VM_NAME" --name "$SATA_CTL" --add sata --controller IntelAhci
        "$VBOX_MANAGE" storageattach "$VM_NAME" --storagectl "$SATA_CTL" --port 0 \
            --device 0 --type hdd --medium "$(realpath "$VDI_IMG")"
        echo -e "${GREEN}[OK]${NC}   VM '$VM_NAME' created."
    fi

    if [ -n "$VBOX_CREATE" ]; then
        echo ""
        echo "VM '$VM_NAME' is ready. Start it with:"
        echo "  $VBOX_MANAGE startvm \"$VM_NAME\""
        echo ""
        echo "SSH:  ssh -i output/ssh-test-key root@localhost -p $SSH_PORT"
        exit 0
    fi

    echo -e "${BLUE}[INFO]${NC} Starting VM '$VM_NAME'..."
    echo "  SSH:  ssh -i output/ssh-test-key root@localhost -p $SSH_PORT"
    echo "  Pass: ssh root@localhost -p $SSH_PORT  (password: veiladmin)"
    exec "$VBOX_MANAGE" startvm "$VM_NAME"
fi

if [ -z "$BIOS" ] && [ ! -f "$KERNEL" ]; then
    echo -e "${RED}ERROR${NC}: Kernel not found at $KERNEL"
    echo "Run ./build.sh first."
    exit 1
fi

if [ ! -f "$DISK_IMG" ]; then
    if [ -f "$VDI_IMG" ]; then
        echo -e "${YELLOW}Converting${NC} $VDI_IMG -> $DISK_IMG ..."
        $QEMU_IMG convert -f vdi -O raw "$VDI_IMG" "$DISK_IMG" || {
            echo -e "${RED}ERROR${NC}: Failed to convert VDI to raw"
            exit 1
        }
    else
        echo -e "${RED}ERROR${NC}: Disk image not found at $DISK_IMG or $VDI_IMG"
        echo "Run ./build.sh first."
        exit 1
    fi
fi

echo ""
echo "============================================"
echo "  Veilbox — QEMU"
echo "============================================"
echo ""
echo "  Kernel:    $KERNEL"
echo "  Disk:      $DISK_IMG (state partition)"
echo "  Memory:    $MEM"
echo "  CPUs:      $SMP"
echo "  SSH:       host:$SSH_PORT -> guest:22"
[ -n "$LOG_FILE" ]  && echo "  Log:       $LOG_FILE"
[ "$TIMEOUT" -gt 0 ] && echo "  Timeout:   ${TIMEOUT}s"
# Create persistent state disk if requested
KERNEL_CMDLINE="console=ttyS0"
if [ -n "$KEEP_STATE" ]; then
    if [ ! -f "$STATE_PERSIST" ]; then
        echo -e "${YELLOW}Creating${NC} persistent state disk: $STATE_PERSIST ..."
        $QEMU_IMG create -f raw "$STATE_PERSIST" 128M > /dev/null
        mkfs.ext4 -F "$STATE_PERSIST" > /dev/null 2>&1
    fi
fi

echo ""
if [ -n "$BIOS" ]; then
    echo "  BIOS/GRUB boot (disk image)"
else
    echo "  Direct kernel boot (embedded initramfs)"
fi
echo "    - BusyBox (init, shell, utilities)"
echo "    - containerd + runc + nerdctl"
echo "    - Dropbear (SSH server)"
[ -n "$AUTOLOGIN" ] && echo "    - Auto-login as root (veilbox.autologin)"
[ -n "$KEEP_STATE" ] && echo "    - Persistent state disk (state-persist.img)"
echo ""
echo "  Login prompt (root/veiladmin) on tty1/ttyS0"
echo ""
echo "  To SSH from another terminal:"
echo "    ssh -i output/ssh-test-key root@localhost -p $SSH_PORT"
echo "    ssh root@localhost -p $SSH_PORT  (password: veiladmin)"
echo ""
echo "  To exit QEMU: Ctrl-A, then X"
echo "============================================"
echo ""

qemu_base=(
    "$QEMU_BINARY"
    -drive file="$DISK_IMG",format=raw,if=virtio
    -m "$MEM"
    -smp "$SMP"
    -nographic
    -no-reboot
    -cpu host
    -enable-kvm
    -netdev user,id=net0,hostfwd=tcp::${SSH_PORT}-:22
    -device virtio-net,netdev=net0
)

if [ -n "$KEEP_STATE" ]; then
    qemu_base+=(-drive file="$STATE_PERSIST",format=raw,if=virtio)
fi

if [ -n "$BIOS" ]; then
    qemu_args=("${qemu_base[@]}" -boot menu=on)
else
    [ -n "$AUTOLOGIN" ] && KERNEL_CMDLINE="$KERNEL_CMDLINE veilbox.autologin"
    qemu_args=("${qemu_base[@]}" -kernel "$KERNEL" -append "$KERNEL_CMDLINE")
fi

if [ -n "$CHECK" ]; then
    # Auto-enable autologin for more thorough check
    AUTOLOGIN=1
    # BIOS/GRUB mode needs more time for SeaBIOS + GRUB menu countdown
    [ -n "$BIOS" ] && TIMEOUT=60 || TIMEOUT=45
    local_log="${LOG_FILE:-$(mktemp /tmp/qemu-serial-XXXXXX)}"
    [ -z "$LOG_FILE" ] && trap "rm -f '$local_log'" EXIT
    # Rebuild args with autologin enabled
    if [ -z "$BIOS" ]; then
        KERNEL_CMDLINE="$KERNEL_CMDLINE veilbox.autologin"
        qemu_args=("${qemu_base[@]}" -kernel "$KERNEL" -append "$KERNEL_CMDLINE")
    fi
    timeout "$TIMEOUT" "${qemu_args[@]}" > "$local_log" 2>&1 || true
    if grep -q 'root@veilbox' "$local_log" 2>/dev/null || grep -q 'veilbox login:' "$local_log" 2>/dev/null; then
        echo -e "${GREEN}[OK]${NC}   VM booted successfully (login prompt detected)"
        exit 0
    else
        echo -e "${RED}[ERROR]${NC} VM failed to reach login prompt"
        echo "Last 20 lines of serial output:"
        tail -20 "$local_log" 2>/dev/null || true
        exit 1
    fi
fi

if [ -n "$LOG_FILE" ]; then
    "${qemu_args[@]}" 2>&1 | tee "$LOG_FILE"
    exit "${PIPESTATUS[0]}"
elif [ "$TIMEOUT" -gt 0 ]; then
    exec timeout "$TIMEOUT" "${qemu_args[@]}"
else
    exec "${qemu_args[@]}"
fi
