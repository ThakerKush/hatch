export type VMState =
  | "starting"
  | "running"
  | "stopping"
  | "stopped"
  | "snapshotted"
  | "error";

export interface VM {
  id: string;
  image_id: string;
  template_id?: string;
  state: VMState;
  vcpu_count: number;
  mem_mib: number;
  enable_network: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateVMOptions {
  image_id?: string;
  template_id?: string;
  vcpu_count?: number;
  mem_mib?: number;
  boot_args?: string;
  enable_network?: boolean;
  user_data?: string;
}

export interface Snapshot {
  id: string;
  vm_id: string;
  state_key: string;
  memory_key: string;
  disk_key: string;
  vm_config: string;
  size_bytes: number;
  created_at: string;
}

export interface ProxyRoute {
  id: string;
  vm_id: string;
  subdomain: string;
  target_port: number;
  auto_wake: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateRouteOptions {
  subdomain?: string;
  target_port: number;
  auto_wake?: boolean;
}

export interface ExecOptions {
  /** Timeout in milliseconds. Defaults to 10000 (10s) on the server. */
  timeout_ms?: number;
  /** Run the command in the background. Returns immediately with pid and log_file. */
  background?: boolean;
}

export interface ExecResult {
  stdout: string;
  stderr: string;
  exit_code: number;
  timed_out: boolean;
  /** PID of the background/timed-out process. */
  pid?: number;
  /** Log file path on the VM with the process output. */
  log_file?: string;
}

export interface NetMetrics {
  rx_bytes: number;
  tx_bytes: number;
  rx_packets: number;
  tx_packets: number;
}

export interface BlockMetrics {
  read_bytes: number;
  write_bytes: number;
}

export interface VCPUMetrics {
  cpu_time_micros: number;
  utilization_percent: number;
}

export interface VMMetrics {
  vm_id: string;
  state: string;
  vcpu_count: number;
  mem_mib: number;
  uptime_seconds: number;
  net: NetMetrics;
  block: BlockMetrics;
  vcpu: VCPUMetrics;
  raw: Record<string, unknown>;
}

export interface HealthStatus {
  status: string;
  vm_count: number;
  route_count: number;
}

export interface HatchClientOptions {
  apiKey: string;
  /** Override the API base URL. Defaults to https://api.hatchvm.com */
  apiUrl?: string;
}
