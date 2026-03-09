export { HatchClient, HatchError } from "./client.js";
export type {
  VM,
  VMState,
  CreateVMOptions,
  Snapshot,
  ProxyRoute,
  CreateRouteOptions,
  ExecOptions,
  ExecResult,
  NetMetrics,
  BlockMetrics,
  VCPUMetrics,
  VMMetrics,
  HealthStatus,
  HatchClientOptions,
} from "./types.js";

import { HatchClient } from "./client.js";
import type { HatchClientOptions } from "./types.js";

/**
 * Create a new HatchClient instance.
 *
 * @example
 * ```ts
 * import { create } from "hatchvm";
 *
 * const hatch = create({ apiKey: "your-api-key" });
 *
 * const vm = await hatch.createVM({ vcpu_count: 2, mem_mib: 512 });
 * const result = await hatch.exec(vm.id, "echo hello");
 * console.log(result.stdout);
 * ```
 */
export function create(opts: HatchClientOptions): HatchClient {
  return new HatchClient(opts);
}
