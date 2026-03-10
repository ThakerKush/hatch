# hatchvm

TypeScript SDK for the [Hatch](https://hatchvm.com) microVM platform. Spin up Firecracker microVMs, execute commands, and expose services.

## Install

```bash
npm install hatchvm
```

## Quick start

```ts
import { create } from "hatchvm";

const hatch = create({ apiKey: "your-api-key" });

// Create a VM
const vm = await hatch.createVM({ vcpu_count: 2, mem_mib: 512 });

// Run a command
const result = await hatch.exec(vm.id, "echo hello world");
console.log(result.stdout); // "hello world\n"

// Expose a port to the internet
const route = await hatch.createRoute(vm.id, { target_port: 8080 });
console.log(`Live at: https://${route.subdomain}.hatchvm.com`);
```

## VM lifecycle

```ts
// Create with cloud-init user data
const vm = await hatch.createVM({
  vcpu_count: 2,
  mem_mib: 512,
  user_data: `#cloud-config
packages:
  - python3
`,
});

// List all VMs
const vms = await hatch.listVMs();

// Get a specific VM
const info = await hatch.getVM(vm.id);

// Stop and delete
await hatch.stopVM(vm.id);
await hatch.deleteVM(vm.id);
```

## Command execution

Commands run inside the VM via SSH. By default they have a 10 second timeout.

```ts
// Basic command
const result = await hatch.exec(vm.id, "ls -la /home");

// Custom timeout (30 seconds)
const build = await hatch.exec(vm.id, "make build", { timeout_ms: 30000 });

// Background mode — returns immediately with pid and log file
const server = await hatch.exec(vm.id, "python3 -m http.server 8080", {
  background: true,
});
console.log(server.pid);      // PID of the background process
console.log(server.log_file); // Path to stdout/stderr log on the VM

// Read background process output
const logs = await hatch.exec(vm.id, `cat ${server.log_file}`);
```

### Foreground vs background

| Mode | Behavior |
|------|----------|
| **Foreground** (default) | Blocks until the command finishes or `timeout_ms` is reached. If it times out, `timed_out: true` is set and the process keeps running in the VM. |
| **Background** | Returns immediately with `pid` and `log_file`. The process runs detached in the VM. Poll the log file to check output. |

## Routing

Expose VM ports to the internet via `*.hatchvm.com` subdomains.

```ts
// Auto-generated subdomain
const route = await hatch.createRoute(vm.id, { target_port: 3000 });

// Custom subdomain
const route = await hatch.createRoute(vm.id, {
  target_port: 8080,
  subdomain: "my-app",
});
// => https://my-app.hatchvm.com

// List and delete routes
const routes = await hatch.listRoutes(vm.id);
await hatch.deleteRoute(route.id);
```

## Snapshots

Snapshot a running VM and restore it later.

```ts
const snapshot = await hatch.snapshotVM(vm.id);
const restored = await hatch.restoreVM(vm.id);
const snapshots = await hatch.listSnapshots(vm.id);
```

## Metrics

```ts
const metrics = await hatch.getMetrics(vm.id);
console.log(metrics.uptime_seconds);
console.log(metrics.net.rx_bytes);
console.log(metrics.vcpu.utilization_percent);
```

## Error handling

All API errors throw a `HatchError` with the HTTP status code.

```ts
import { HatchError } from "hatchvm";

try {
  await hatch.getVM("nonexistent");
} catch (err) {
  if (err instanceof HatchError) {
    console.log(err.message); // Error message from the API
    console.log(err.status);  // HTTP status code (e.g. 404)
  }
}
```

## Requirements

- Node.js 18+
- No runtime dependencies

## License

MIT
