# Hatch (V0)

Hatch is a single-tenant Firecracker spawner. The control plane runs on a Linux host with KVM and manages multiple microVMs via the Firecracker API.

## Quick start (Linux host)

1) Install Firecracker and ensure it is in `PATH` (or set `HATCH_FIRECRACKER_BIN`).
2) Run the server:

```
go run ./cmd/hatchd
```

3) Register an image:

```
curl -sS -X POST localhost:8080/images \
  -H 'content-type: application/json' \
  -d '{
    "kernel_path": "/path/to/vmlinux.bin",
    "rootfs_path": "/path/to/rootfs.ext4",
    "boot_args": "console=ttyS0 reboot=k panic=1 pci=off root=/dev/vda rw rootfstype=ext4"
  }'
```

4) Create a VM:

```
curl -sS -X POST localhost:8080/vms \
  -H 'content-type: application/json' \
  -d '{"image_id":"<image-id>"}'
```

## Networking notes

Hatch creates a Linux bridge (`fcbr0` by default) and attaches per-VM tap devices to it. The API allocates a guest IP, but the guest must be configured to use it (via static config or your own DHCP setup in the guest).

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

