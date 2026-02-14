# Hatch

Hatch is a single-tenant Firecracker control plane for spinning up microVMs — designed for AI agent workloads. It provides a REST API for VM lifecycle management, a subdomain-based reverse proxy with wake-on-request, automatic snapshot/restore for idle VMs, and reusable VM templates.

## Features

- **VM lifecycle** — create, stop, delete microVMs via REST API
- **Automatic networking** — bridge + TAP + DHCP + cloud-init, fully automated
- **Templates** — reusable VM configs (image + cloud-init + resource defaults)
- **Reverse proxy** — subdomain-based routing (`agent.hatch.local` → VM guest IP)
- **Wake-on-request** — snapshotted VMs auto-restore when traffic arrives
- **Snapshot / restore** — Firecracker native snapshots stored in S3-compatible storage
- **Idle detection** — VMs with no proxy traffic get auto-snapshotted after a configurable timeout
- **SSH access ports** — each networked VM gets a host `ssh_port` forwarded to guest `:22`
- **PostgreSQL** — persistent state for VMs, images, templates, snapshots, routes
- **Docker Compose** — one command to run Hatch + Postgres + MinIO

## Quick start

### Docker Compose (recommended)

On your Linux VPS with KVM and Firecracker installed:

```bash
git clone <your-repo> && cd hatch
docker compose up -d
```

This starts:
- **Hatch** daemon (API on `:8080`, proxy on `:9090`)
- **PostgreSQL** on `:5432`
- **MinIO** (S3-compatible) on `:9000` (console on `:9001`)

### Manual (Linux host)

Prerequisites: `firecracker`, `dnsmasq-base`, `e2fsprogs`, a running PostgreSQL.

```bash
export DATABASE_URL="postgres://hatch:hatch@localhost:5432/hatch?sslmode=disable"
go run ./cmd/hatchd
```

## API usage

### Register an image

```bash
curl -sS -X POST localhost:8080/images \
  -H 'content-type: application/json' \
  -d '{
    "kernel_path": "/path/to/vmlinux.bin",
    "rootfs_path": "/path/to/rootfs.ext4"
  }'
```

### Create a template

```bash
curl -sS -X POST localhost:8080/templates \
  -H 'content-type: application/json' \
  -d '{
    "name": "vscode-agent",
    "image_id": "<image-id>",
    "vcpu_count": 2,
    "mem_mib": 1024,
    "user_data": "#cloud-config\npackages:\n  - python3\nruncmd:\n  - curl -fsSL https://code-server.dev/install.sh | sh"
  }'
```

### Create a VM (from template)

```bash
curl -sS -X POST localhost:8080/vms \
  -H 'content-type: application/json' \
  -d '{"template_id": "<template-id>"}'
```

The response includes `ssh_port` for networked VMs. Connect with:

```bash
ssh -p <ssh_port> <user>@<hatch-host-ip>
```

### Set up a proxy route

```bash
curl -sS -X POST localhost:8080/vms/<vm-id>/routes \
  -H 'content-type: application/json' \
  -d '{"subdomain": "my-agent", "target_port": 8080}'
```

Now `http://my-agent.hatch.local:9090/` forwards to the VM's port 8080.

### Snapshot / restore

```bash
# Manual snapshot
curl -sS -X POST localhost:8080/vms/<vm-id>/snapshot

# Manual restore
curl -sS -X POST localhost:8080/vms/<vm-id>/restore

# List snapshots
curl -sS localhost:8080/vms/<vm-id>/snapshots
```

Idle VMs with proxy routes are auto-snapshotted after `HATCH_IDLE_TIMEOUT` (default 10m). When a request arrives for a snapshotted VM, the proxy auto-restores it.

## API endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/healthz` | Health check with VM/route counts |
| POST | `/images` | Register an image |
| GET | `/images` | List images |
| GET | `/images/{id}` | Get image |
| DELETE | `/images/{id}` | Delete image |
| POST | `/templates` | Create template |
| GET | `/templates` | List templates |
| GET | `/templates/{id}` | Get template |
| DELETE | `/templates/{id}` | Delete template |
| POST | `/vms` | Create VM (accepts `image_id` or `template_id`) |
| GET | `/vms` | List VMs |
| GET | `/vms/{id}` | Get VM |
| DELETE | `/vms/{id}` | Delete VM |
| POST | `/vms/{id}/stop` | Stop VM |
| POST | `/vms/{id}/snapshot` | Snapshot VM to S3 |
| POST | `/vms/{id}/restore` | Restore VM from latest snapshot |
| GET | `/vms/{id}/snapshots` | List snapshots for a VM |
| POST | `/vms/{id}/routes` | Create proxy route |
| GET | `/vms/{id}/routes` | List proxy routes |
| DELETE | `/routes/{id}` | Delete proxy route |

## Networking

Hatch creates a Linux bridge (`fcbr0` by default) and attaches per-VM TAP devices.

1. On first VM, Hatch starts `dnsmasq` DHCP on the bridge.
2. Each VM gets an allocated IP with a static DHCP reservation (MAC -> IP).
3. A cloud-init NoCloud seed disk configures DHCP on the guest NIC.
4. Guest boots, cloud-init runs, network is up — fully automatic.

For internet access from VMs:

```bash
sudo sysctl -w net.ipv4.ip_forward=1
sudo iptables -t nat -A POSTROUTING -s 172.16.0.0/24 ! -o fcbr0 -j MASQUERADE
```

## Image strategy

Hatch images are simple pointers to kernel + rootfs files. Hatch does not build images.

- **Day 1:** Build rootfs externally (Dockerfile-to-ext4, debootstrap, cloud image download) and register via `POST /images`.
- **Day 2:** Create templates that bundle image + cloud-init + defaults. One API call spins up a pre-configured VM.
- **Day 3:** Boot a VM from a template, wait for cloud-init, snapshot it. Restore copies for instant boot with everything pre-installed (golden snapshot pattern).

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `HATCH_HTTP_ADDR` | `:8080` | API listen address |
| `HATCH_PROXY_ADDR` | `:9090` | Reverse proxy listen address |
| `HATCH_PROXY_BASE_DOMAIN` | `hatch.local` | Base domain for subdomain routing |
| `HATCH_PROXY_WAKE_TIMEOUT` | `60s` | Max time to wait for VM restore |
| `HATCH_SSH_PORT_MIN` | `16000` | Minimum host port for VM SSH forwarding |
| `HATCH_SSH_PORT_MAX` | `26000` | Maximum host port for VM SSH forwarding |
| `HATCH_SSH_ALLOWED_CIDR` | `127.0.0.1/32` | Source CIDR allowed to reach forwarded SSH ports |
| `HATCH_DATA_DIR` | `./data` | Local data directory |
| `DATABASE_URL` | `postgres://hatch:hatch@localhost:5432/hatch?sslmode=disable` | PostgreSQL connection string |
| `HATCH_FIRECRACKER_BIN` | `firecracker` | Firecracker binary path |
| `HATCH_BRIDGE_NAME` | `fcbr0` | Bridge interface name |
| `HATCH_BRIDGE_CIDR` | `172.16.0.1/24` | Bridge IP/subnet |
| `HATCH_DEFAULT_VCPU` | `1` | Default vCPUs per VM |
| `HATCH_DEFAULT_MEM_MIB` | `256` | Default memory (MiB) per VM |
| `HATCH_S3_ENDPOINT` | | S3 endpoint (e.g. `http://localhost:9000` for MinIO) |
| `HATCH_S3_BUCKET` | | S3 bucket for snapshots |
| `HATCH_S3_REGION` | `us-east-1` | S3 region |
| `HATCH_S3_ACCESS_KEY` | | S3 access key |
| `HATCH_S3_SECRET_KEY` | | S3 secret key |
| `HATCH_IDLE_CHECK_INTERVAL` | `1m` | How often to check for idle VMs |
| `HATCH_IDLE_TIMEOUT` | `10m` | Idle time before auto-snapshot |

