#!/usr/bin/env bash
set -euo pipefail

[ "$EUID" -eq 0 ] || { echo "Run as root: sudo $0"; exit 1; }

ROOTFS="${1:-/root/firecracker/ubuntu-noble-rootfs.ext4}"
ARCH="$(uname -m)"

if [ ! -f "$ROOTFS" ]; then
    echo "Rootfs not found at $ROOTFS"
    echo "Usage: sudo $0 [path-to-rootfs.ext4]"
    echo ""
    echo "If you haven't downloaded the base image yet, run install-deps.sh first."
    exit 1
fi

# ── Re-download a clean base image if requested ──────────────────────────────

if [ "${REDOWNLOAD:-}" = "1" ]; then
    echo "Re-downloading clean base image..."
    wget -q --show-progress -O "$ROOTFS" \
        "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-${ARCH}.img"
    echo "Clean base image downloaded."
fi

# ── Shrink the image if it's larger than the target ──────────────────────────

TARGET_MIB="${TARGET_SIZE_MIB:-2048}"
CURRENT_BYTES=$(stat --printf="%s" "$ROOTFS" 2>/dev/null || stat -f "%z" "$ROOTFS")
CURRENT_MIB=$((CURRENT_BYTES / 1024 / 1024))

if [ "$CURRENT_MIB" -gt "$TARGET_MIB" ]; then
    echo "Base image is ${CURRENT_MIB}M, shrinking to ${TARGET_MIB}M..."
    e2fsck -fy "$ROOTFS" || true
    resize2fs "$ROOTFS" "${TARGET_MIB}M"
    truncate -s "${TARGET_MIB}M" "$ROOTFS"
    echo "Base image shrunk to ${TARGET_MIB}M."
fi

# ── Inject /sbin/overlay-init ────────────────────────────────────────────────

MOUNT_DIR=$(mktemp -d)
trap 'umount "$MOUNT_DIR" 2>/dev/null; rmdir "$MOUNT_DIR" 2>/dev/null' EXIT

mount -o loop "$ROOTFS" "$MOUNT_DIR"

cat > "${MOUNT_DIR}/sbin/overlay-init" <<'INITEOF'
#!/bin/sh
set -eu

if [ ! -b /dev/vdb ]; then
    exec /sbin/init "$@"
fi

mkdir -p /mnt/overlay /mnt/merged

mount -t ext4 /dev/vdb /mnt/overlay
mkdir -p /mnt/overlay/upper /mnt/overlay/work

mount -t overlay overlay \
    -o lowerdir=/,upperdir=/mnt/overlay/upper,workdir=/mnt/overlay/work \
    /mnt/merged

mkdir -p /mnt/merged/.overlay-backing
mount --move /mnt/overlay /mnt/merged/.overlay-backing

cd /mnt/merged
mkdir -p .pivot-old
pivot_root . .pivot-old

mount -t proc proc /proc 2>/dev/null || true
mount -t sysfs sysfs /sys 2>/dev/null || true
mount -t devtmpfs devtmpfs /dev 2>/dev/null || true
mkdir -p /dev/pts /dev/shm /run
mount -t devpts devpts /dev/pts 2>/dev/null || true
mount -t tmpfs tmpfs /dev/shm 2>/dev/null || true
mount -t tmpfs tmpfs /run 2>/dev/null || true

umount -l /.pivot-old 2>/dev/null || true
exec /sbin/init "$@"
INITEOF

chmod 0755 "${MOUNT_DIR}/sbin/overlay-init"
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
