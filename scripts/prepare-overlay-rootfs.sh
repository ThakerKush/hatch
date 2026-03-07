#!/usr/bin/env bash
set -euo pipefail

[ "$EUID" -eq 0 ] || { echo "Run as root: sudo $0"; exit 1; }

ROOTFS="${1:-/root/firecracker/ubuntu-noble-rootfs.ext4}"

# Ubuntu cloud images use "amd64" / "arm64", not uname's "x86_64" / "aarch64".
case "$(uname -m)" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    *)       ARCH="$(uname -m)" ;;
esac

echo "[1/5] Config"
echo "  ROOTFS=$ROOTFS"
echo "  ARCH=$ARCH"
echo "  REDOWNLOAD=${REDOWNLOAD:-0}"

# ── Re-download a clean base image if requested ──────────────────────────────

if [ "${REDOWNLOAD:-}" = "1" ]; then
    mkdir -p "$(dirname "$ROOTFS")"
    QCOW2_TMP="$(dirname "$ROOTFS")/base-download.qcow2"

    echo ""
    echo "[2/5] Downloading clean base image..."
    URL="https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-${ARCH}.img"
    echo "  URL=$URL"
    wget -v -O "$QCOW2_TMP" "$URL"
    echo "  Download complete: $(ls -lh "$QCOW2_TMP" | awk '{print $5}')"

    echo ""
    echo "[3/5] Converting QCOW2 → bare ext4 root partition..."
    if ! command -v qemu-img &>/dev/null; then
        echo "  qemu-img not found, installing qemu-utils..."
        apt-get update -qq && apt-get install -y -qq qemu-utils
    fi

    RAW_TMP="$(dirname "$ROOTFS")/base-download.raw"
    qemu-img convert -f qcow2 -O raw "$QCOW2_TMP" "$RAW_TMP"
    rm -f "$QCOW2_TMP"
    echo "  Raw disk: $(ls -lh "$RAW_TMP" | awk '{print $5}')"

    # The raw image is a full GPT disk. Extract just the root (ext4) partition.
    LOOP_DEV=$(losetup --find --show --partscan "$RAW_TMP")
    echo "  Loop device: $LOOP_DEV"

    # Find the largest partition (the root fs, not the EFI boot partition).
    ROOT_PART=""
    LARGEST=0
    for part in "${LOOP_DEV}p"*; do
        [ -b "$part" ] || continue
        PSIZE=$(blockdev --getsize64 "$part")
        echo "  Found partition: $part ($(( PSIZE / 1024 / 1024 ))M)"
        if [ "$PSIZE" -gt "$LARGEST" ]; then
            LARGEST=$PSIZE
            ROOT_PART=$part
        fi
    done

    if [ -z "$ROOT_PART" ]; then
        losetup -d "$LOOP_DEV"
        rm -f "$RAW_TMP"
        echo "  ERROR: no partitions found in disk image"
        exit 1
    fi

    echo "  Extracting root partition: $ROOT_PART"
    dd if="$ROOT_PART" of="$ROOTFS" bs=4M status=progress
    losetup -d "$LOOP_DEV"
    rm -f "$RAW_TMP"
    echo "  Extracted: $(ls -lh "$ROOTFS" | awk '{print $5}')"
else
    echo ""
    echo "[2/5] Skipping download (REDOWNLOAD not set)"
    echo "[3/5] Skipping QCOW2 conversion"
fi

if [ ! -f "$ROOTFS" ]; then
    echo ""
    echo "ERROR: Rootfs not found at $ROOTFS"
    echo "Usage: sudo REDOWNLOAD=1 $0 [path-to-rootfs.ext4]"
    echo ""
    echo "If you haven't downloaded the base image yet, run install-deps.sh first"
    echo "or use REDOWNLOAD=1."
    exit 1
fi

# ── Shrink the image if it's larger than the target ──────────────────────────

TARGET_MIB="${TARGET_SIZE_MIB:-3072}"
CURRENT_BYTES=$(stat --printf="%s" "$ROOTFS" 2>/dev/null || stat -f "%z" "$ROOTFS")
CURRENT_MIB=$((CURRENT_BYTES / 1024 / 1024))

echo ""
echo "[4/5] Size check"
echo "  Current: ${CURRENT_MIB}M"
echo "  Target:  ${TARGET_MIB}M"

if [ "$CURRENT_MIB" -gt "$TARGET_MIB" ]; then
    echo "  Shrinking..."
    e2fsck -fy "$ROOTFS" || true
    if resize2fs "$ROOTFS" "${TARGET_MIB}M" 2>/dev/null; then
        truncate -s "${TARGET_MIB}M" "$ROOTFS"
        echo "  Shrunk to ${TARGET_MIB}M."
    else
        echo "  Filesystem minimum is larger than ${TARGET_MIB}M, keeping as-is (${CURRENT_MIB}M)."
    fi
else
    echo "  No shrink needed."
fi

# ── Inject /sbin/overlay-init ────────────────────────────────────────────────

echo ""
echo "[5/5] Injecting /sbin/overlay-init"

MOUNT_DIR=$(mktemp -d)
echo "  Mount dir: $MOUNT_DIR"
trap 'umount "$MOUNT_DIR" 2>/dev/null; rmdir "$MOUNT_DIR" 2>/dev/null' EXIT

mount -o loop "$ROOTFS" "$MOUNT_DIR"
echo "  Mounted $ROOTFS"

# Pre-create mount points so the init script works on a read-only root.
mkdir -p "${MOUNT_DIR}/mnt/overlay" "${MOUNT_DIR}/mnt/merged"
echo "  Pre-created /mnt/overlay and /mnt/merged in base image"

# Remove fstab entries for partitions that don't exist in the Firecracker VM.
# The base image came from a full GPT disk with UEFI and BOOT partitions that
# we stripped out. If left in fstab, systemd waits 90s per missing device and
# then drops to emergency mode.
if [ -f "${MOUNT_DIR}/etc/fstab" ]; then
    echo "  Cleaning /etc/fstab..."
    echo "  Before:"
    cat "${MOUNT_DIR}/etc/fstab" | sed 's/^/    /'
    sed -i '/LABEL=UEFI/d; /LABEL=BOOT/d; /by-label\/UEFI/d; /by-label\/BOOT/d' "${MOUNT_DIR}/etc/fstab"
    echo "  After:"
    cat "${MOUNT_DIR}/etc/fstab" | sed 's/^/    /'
fi

cat > "${MOUNT_DIR}/sbin/overlay-init" <<'INITEOF'
#!/bin/sh
# Early init script that sets up OverlayFS before handing off to systemd.
# Runs on a read-only root, so /etc/mtab updates will fail -- use -n flag
# on all mount calls to skip mtab writes.

if [ ! -b /dev/vdb ]; then
    exec /sbin/init "$@"
fi

mount -n -t ext4 /dev/vdb /mnt/overlay
mkdir -p /mnt/overlay/upper /mnt/overlay/work

mount -n -t overlay overlay \
    -o lowerdir=/,upperdir=/mnt/overlay/upper,workdir=/mnt/overlay/work \
    /mnt/merged

mkdir -p /mnt/merged/.overlay-backing
mount -n --move /mnt/overlay /mnt/merged/.overlay-backing

cd /mnt/merged
mkdir -p .pivot-old
pivot_root . .pivot-old

mount -n -t proc proc /proc 2>/dev/null || true
mount -n -t sysfs sysfs /sys 2>/dev/null || true
mount -n -t devtmpfs devtmpfs /dev 2>/dev/null || true
mkdir -p /dev/pts /dev/shm /run
mount -n -t devpts devpts /dev/pts 2>/dev/null || true
mount -n -t tmpfs tmpfs /dev/shm 2>/dev/null || true
mount -n -t tmpfs tmpfs /run 2>/dev/null || true

umount -l /.pivot-old 2>/dev/null || true
exec /sbin/init "$@"
INITEOF

chmod 0755 "${MOUNT_DIR}/sbin/overlay-init"
echo "  Written: $(ls -l "${MOUNT_DIR}/sbin/overlay-init")"
sync

umount "$MOUNT_DIR"
trap - EXIT
rmdir "$MOUNT_DIR"

echo ""
echo "Done. /sbin/overlay-init has been injected into $ROOTFS"
echo ""
echo "The base image is ready for the overlay boot path."
echo "Make sure your .env has:"
echo ""
echo "  HATCH_DEFAULT_ROOTFS_PATH=$ROOTFS"
echo ""
