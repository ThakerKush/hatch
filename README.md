# Hatch
Hatch is a wrapper around Firecracker for spnning up microVMs, designed for agentic workloads, with a REST api for lifecycle mangement, wake-on-request, snapshot/restore for idel VMs and subdomain-based reverse proxy. 

### Setup: 

**Prerequisites**
- Linux machine with KVM enabled (`ls /dev/kvm && echo "KVM enabled" || echo "KVM not enabled"`)
- Dependencies Installed ( run `hatch/scripts/install-deps.sh` to install dependencies )
- Postgres 
- S3-compatable storage if you want snapshot/restore

`scripts/install-deps.sh` installs Firecracker, dnsmasq, kernel, rootfs, and system networking tools, then prints the env vars to paste into .env.

**Run** 
```bash
#docker compose brings up Postgres + MinIO
docker compose up -d

##hatch 
sudo go run ./cmd/hatchd
```

**First VM**
```bash 
curl -X POST localhost:8080/vms \
  -H 'content-type: application/json' \
  -d '{
    "enable_network": true,
    "user_data": "#cloud-config\nusers:\n  - name: hatch\n    groups: [sudo]\n    shell: /bin/bash\n    sudo: [\"ALL=(ALL) NOPASSWD:ALL\"]\n    ssh_authorized_keys:\n      - <your-public-key>"
  }'

  ssh -p <ssh_port> hatch@<host-ip>
```


### Architecture overview

![architecture](/docs/images/architecture.png)


### Networking — bridge, TAP, and DHCP

When a networked VM is created, Hatch sets up a full Linux networking stack on the host before Firecracker ever starts:

1. **Bridge (`fcbr0`)** — a Layer 2 virtual switch created once, shared across all VMs. It gets the gateway IP (`172.16.0.1`) and is where `dnsmasq` listens to serve DHCP.

2. **TAP device (`fctap-<vmid>`)** — one per VM, created on the host and plugged into the bridge as a port. Firecracker holds the other end as a file descriptor and uses it to send and receive raw Ethernet frames. From the guest's perspective it looks like a regular NIC (`eth0`).

3. **IP + MAC allocation** — Hatch generates a random MAC and picks the next free IP from the bridge subnet, entirely on the host before the VM starts.

4. **DHCP reservation** — the MAC → IP mapping is written into dnsmasq's hosts file and dnsmasq is signalled (`SIGHUP`) to reload. The DHCP is deterministic: the IP is pre-decided on the host, dnsmasq just delivers it to the guest.

5. **cloud-init injection** — Hatch loop-mounts the VM's rootfs image on the host and writes a `network-config` file into `/var/lib/cloud/seed/nocloud/` inside it:
   ```yaml
   version: 2
   ethernets:
     eth0:
       match:
         macaddress: "aa:bb:cc:dd:ee:ff"
       dhcp4: true
   ```
   When the guest boots, cloud-init finds this file and runs DHCP on `eth0`. The request travels `eth0 → TAP → bridge → dnsmasq`, and the guest gets back exactly the IP Hatch pre-allocated. No manual configuration inside the guest.

6. **NAT** — iptables MASQUERADE rule on the bridge subnet lets VMs reach the internet through the host's real NIC.

```
DHCP flow:

  guest eth0 ──► TAP fctap-xxxx ──► bridge fcbr0 ──► dnsmasq
                                                         │
                                      DHCPACK ◄──────────┘
  guest gets: IP 172.16.0.10, GW 172.16.0.1, DNS 8.8.8.8
```

### SSH forwarding

Each networked VM gets a dedicated host port (in the range `HATCH_SSH_PORT_MIN`–`HATCH_SSH_PORT_MAX`) forwarded to guest `:22` via iptables DNAT:

```
ssh -p 16000 user@host
  ─► host:16000
  ─► iptables PREROUTING DNAT ─► 172.16.0.10:22
  ─► bridge ─► TAP ─► VM eth0 ─► sshd
```

The SSH gateway also handles **wake-on-SSH**: if a VM is snapshotted when you try to connect, Hatch restores it first and then forwards your connection — your SSH client just sees a slow handshake.

### Subdomain reverse proxy

Hatch runs a second HTTP server (`:9090`) that routes incoming requests to VMs by subdomain. You register a route per VM:

```bash
POST /vms/<id>/routes
{ "subdomain": "my-agent", "target_port": 3000, "auto_wake": true }
```

Any request to `my-agent.hatch.local` is then reverse-proxied to `172.16.0.x:3000` inside that VM. The proxy extracts the subdomain, looks up the route in Postgres, gets the VM's guest IP, and forwards. Multiple routes per VM are supported (different subdomains, different ports).

### Wake-on-request

If `auto_wake: true` and the VM is snapshotted, the proxy doesn't return an error — it wakes the VM first (restores from S3 snapshot), then forwards the request. Concurrent wake requests for the same VM are serialised so only one restore runs at a time. This is the core serverless-VM pattern: freeze idle VMs to zero compute, wake them transparently on the next request.

### Idle auto-snapshot

The idle monitor runs a background loop every `HATCH_IDLE_CHECK_INTERVAL`. For each VM that has a proxy route, it tracks the last request time. When a VM has been idle longer than `HATCH_IDLE_TIMEOUT`, it is automatically snapshotted and frozen. The monitor skips VMs with active SSH sessions (detected via `/proc/net/nf_conntrack`) to avoid interrupting live work.

```
idle timer fires
  → check last proxy request time per subdomain
  → if idle > HATCH_IDLE_TIMEOUT and no active SSH
  → snapshot VM to S3 → stop Firecracker process
  → VM is now frozen at zero compute cost
  → next HTTP request or SSH connection wakes it automatically
```

### Snapshots

Snapshots capture the full VM state — CPU registers, memory, and a disk delta — and upload them to S3-compatible storage. Restore downloads and replays them into a fresh Firecracker process. The VM resumes from exactly where it was paused, with the same IP, MAC, and SSH port.

```
Snapshot:  pause VM → dump memory + vmstate → diff rootfs → upload to S3 → kill Firecracker
Restore:   download from S3 → apply disk delta → new Firecracker process → load snapshot → resume
```


---

## API reference

| Method | Path | Description |
|--------|------|-------------|
| GET | `/healthz` | Health check with VM and route counts |
| POST | `/images` | Register a kernel + rootfs image |
| GET | `/images` | List images |
| GET | `/images/{id}` | Get image |
| DELETE | `/images/{id}` | Delete image |
| POST | `/templates` | Create a template |
| GET | `/templates` | List templates |
| GET | `/templates/{id}` | Get template |
| DELETE | `/templates/{id}` | Delete template |
| POST | `/vms` | Create and start a VM |
| GET | `/vms` | List all VMs |
| GET | `/vms/{id}` | Get VM |
| DELETE | `/vms/{id}` | Delete VM and release all resources |
| POST | `/vms/{id}/stop` | Stop a running VM |
| POST | `/vms/{id}/snapshot` | Snapshot VM to S3 |
| POST | `/vms/{id}/restore` | Restore VM from latest snapshot |
| GET | `/vms/{id}/snapshots` | List snapshots for a VM |
| POST | `/vms/{id}/routes` | Create a proxy route |
| GET | `/vms/{id}/routes` | List proxy routes for a VM |
| DELETE | `/routes/{id}` | Delete a proxy route |

---

## Roadmap

**Performance**
- Local snapshot cache to skip S3 round-trips on warm restores
- Diff/incremental snapshots (upload only changed memory pages)
- Target: sub-5s restore for warm cache, sub-15s cold

**Networking**
- Per-VM egress bandwidth limits (tc/qdisc)
- IPv6 support on the bridge
- WireGuard overlay for multi-node clusters
- Per-VM DNS records (`<vmid>.internal`)

**Multi-tenancy & auth**
- API key / JWT authentication
- Per-tenant resource quotas and isolated bridges

**Scheduler & multi-node**
- Distribute VMs across multiple hosts
- Live migration using snapshot/restore
- Placement strategies (bin-packing, spread, GPU affinity)

**Hypervisor**
- Cloud-Hypervisor support (PCI passthrough, GPU, larger VMs)
- OCI image support
- Unified API — same endpoints, hypervisor choice per template

**Developer experience**
- CLI (`hatch create`, `hatch ssh`, `hatch snapshot`)
- Web dashboard
- WebSocket terminal (browser SSH)
- TypeScript SDK
- Pre-built golden images (Ubuntu, Debian, Alpine)

**Observability**
- Prometheus metrics (VM count, snapshot/restore latency, resource usage)
- Per-VM CPU/memory/network via Firecracker metrics
- Structured log export

**Security**
- Firecracker jailer integration (rootless VMs)
- Encrypted snapshots at rest
- Per-VM egress firewall rules
