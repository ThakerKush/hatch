export type NavTab = "dashboard" | "keys" | "docs";

export type VMState =
  | "starting"
  | "running"
  | "stopping"
  | "stopped"
  | "snapshotted"
  | "error";

export type VMRecord = {
  id: string;
  state: VMState;
  vcpu_count: number;
  mem_mib: number;
  created_at: string;
};

export type VMMetrics = {
  vm_id: string;
  uptime_seconds: number;
  net: {
    rx_bytes: number;
    tx_bytes: number;
  };
  vcpu: {
    utilization_percent: number;
  };
};

export type SnapshotRecord = {
  id: string;
  vm_id: string;
  created_at: string;
};

export type DashboardVM = {
  id: string;
  state: VMState;
  vcpu: number;
  memMib: number;
  cpuPercent: number;
  memPercent: number;
  network: string;
  uptimeLabel: string;
  snapshots: number;
};

export type DashboardStats = {
  activeVMs: number;
  totalVMs: number;
  snapshots: number;
};

export type DashboardApiKey = {
  id: string;
  name: string;
  prefix: string;
  createdAt: string;
  lastUsed: string;
};
