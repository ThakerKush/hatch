# Hatch (V0)

Hatch is a single-tenant Firecracker spawner. The control plane runs on a Linux host with KVM and manages multiple microVMs via the Firecracker API.

## Quick start (Linux host)

1) Install prerequisites:

```
# Firecracker
# Ensure the firecracker binary is in PATH (or set HATCH_FIRECRACKER_BIN).

# dnsmasq (DHCP server for microVMs)
sudo apt install dnsmasq-base   # Debian/Ubuntu
sudo dnf install dnsmasq        # Fedora/RHEL

# e2fsprogs ≥ 1.43 (for mkfs.ext4 -d, used to build cloud-init disks)
# Already installed on most distros.
```

2) Prepare a guest rootfs image **with cloud-init installed**. Hatch injects a
   NoCloud seed disk into each VM, so cloud-init in the guest will automatically
   configure networking via DHCP. Most cloud images (Ubuntu, Debian, Alpine with
   `cloud-init`) work out of the box.

3) Run the server:

```
go run ./cmd/hatchd
```

4) Register an image:

```
curl -sS -X POST localhost:8080/images \
  -H 'content-type: application/json' \
  -d '{
    "kernel_path": "/path/to/vmlinux.bin",
    "rootfs_path": "/path/to/rootfs.ext4",
    "boot_args": "console=ttyS0 reboot=k panic=1 pci=off root=/dev/vda rw rootfstype=ext4"
  }'
```

5) Create a VM:

```
curl -sS -X POST localhost:8080/vms \
  -H 'content-type: application/json' \
  -d '{"image_id":"<image-id>"}'
```

## Networking

Hatch creates a Linux bridge (`fcbr0` by default) and attaches per-VM tap devices to it.

**Automatic IP assignment via DHCP + cloud-init:**

1. When the first VM is created, Hatch starts a `dnsmasq` DHCP server on the bridge.
2. For each VM, Hatch allocates an IP from the bridge subnet and creates a static
   DHCP reservation (MAC → IP) so the guest receives exactly the IP shown in the API
   response.
3. A cloud-init NoCloud seed disk (labelled `cidata`) is generated per-VM and attached
   as a second Firecracker drive (`/dev/vdb`). The seed contains a network-config
   that tells cloud-init to use DHCP on the NIC matching the VM's MAC address.
4. The guest boots, cloud-init discovers the seed disk, and the DHCP client obtains
   the assigned IP — fully automatic, no manual guest configuration needed.

**Requirements:**

- `dnsmasq` (or `dnsmasq-base`) installed on the host.
- The guest rootfs must have **cloud-init** installed and enabled.
- `e2fsprogs` ≥ 1.43 on the host (for `mkfs.ext4 -d`).

**Optional – internet access for VMs:**

If you want VMs to reach the internet through the host, enable IP forwarding
and masquerading:

```
sudo sysctl -w net.ipv4.ip_forward=1
sudo iptables -t nat -A POSTROUTING -s 172.16.0.0/24 ! -o fcbr0 -j MASQUERADE
```

## Config

Environment variables:

- `HATCH_HTTP_ADDR` (default `:8080`)
- `HATCH_DATA_DIR` (default `./data`)
- `HATCH_FIRECRACKER_BIN` (default `firecracker`)
- `HATCH_BRIDGE_NAME` (default `fcbr0`)
- `HATCH_BRIDGE_CIDR` (default `172.16.0.1/24`)
- `HATCH_DEFAULT_VCPU` (default `1`)
- `HATCH_DEFAULT_MEM_MIB` (default `256`)
- `HATCH_DEFAULT_BOOT_ARGS` (default kernel args shown above)

## Dev workflow

- Develop locally on macOS.
- `git push` to your repo.
- SSH into the Linux host, `git pull`, and run `go run ./cmd/hatchd`.

