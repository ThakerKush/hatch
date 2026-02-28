#!/usr/bin/env bash
set -euo pipefail

[ "$EUID" -eq 0 ] || { echo "Run as root: sudo $0"; exit 1; }
[ "$(uname -s)" = "Linux" ] || { echo "Hatch only runs on Linux."; exit 1; }
[ -e /dev/kvm ] || { echo "KVM not available. Check that your host supports virtualisation."; exit 1; }

ARCH="$(uname -m)"
DEST="/root/firecracker"
mkdir -p "$DEST"

# system packages
apt-get update -qq
apt-get install -y -qq dnsmasq-base e2fsprogs iproute2 iptables curl wget ca-certificates util-linux

# ip forwarding
sysctl -w net.ipv4.ip_forward=1 > /dev/null
grep -qxF 'net.ipv4.ip_forward=1' /etc/sysctl.conf || echo 'net.ipv4.ip_forward=1' >> /etc/sysctl.conf

# firecracker binary
if ! command -v firecracker &>/dev/null; then
    LATEST=$(basename $(curl -fsSLI -o /dev/null -w %{url_effective} https://github.com/firecracker-microvm/firecracker/releases/latest))
    TMP=$(mktemp -d)
    curl -fsSL "https://github.com/firecracker-microvm/firecracker/releases/download/${LATEST}/firecracker-${LATEST}-${ARCH}.tgz" \
        | tar -xz -C "$TMP"
    install -m 0755 "${TMP}/release-${LATEST}-${ARCH}/firecracker-${LATEST}-${ARCH}" /usr/local/bin/firecracker
    rm -rf "$TMP"
fi

# kernel (Firecracker CI)
RELEASE_URL="https://github.com/firecracker-microvm/firecracker/releases"
LATEST=${LATEST:-$(basename $(curl -fsSLI -o /dev/null -w %{url_effective} ${RELEASE_URL}/latest))}
CI_VERSION="${LATEST%.*}"
KERNEL_KEY=$(curl -s "http://spec.ccfc.min.s3.amazonaws.com/?prefix=firecracker-ci/${CI_VERSION}/${ARCH}/vmlinux-&list-type=2" \
    | grep -oP "(?<=<Key>)(firecracker-ci/${CI_VERSION}/${ARCH}/vmlinux-[0-9]+\.[0-9]+\.[0-9]+)(?=</Key>)" \
    | sort -V | tail -1)
KERNEL_FILE="${DEST}/$(basename ${KERNEL_KEY})"
[ -f "$KERNEL_FILE" ] || wget -q --show-progress -O "$KERNEL_FILE" "https://s3.amazonaws.com/spec.ccfc.min/${KERNEL_KEY}"

# rootfs — ubuntu noble cloud image (has cloud-init)
ROOTFS_FILE="${DEST}/ubuntu-noble-rootfs.ext4"
[ -f "$ROOTFS_FILE" ] || wget -q --show-progress -O "$ROOTFS_FILE" \
    "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-${ARCH}.img"

echo ""
echo "Done. Add these to your .env:"
echo ""
echo "  HATCH_FIRECRACKER_BIN=/usr/local/bin/firecracker"
echo "  HATCH_DEFAULT_KERNEL_PATH=${KERNEL_FILE}"
echo "  HATCH_DEFAULT_ROOTFS_PATH=${ROOTFS_FILE}"
echo ""
