import type {
  VM,
  CreateVMOptions,
  Snapshot,
  ProxyRoute,
  CreateRouteOptions,
  ExecResult,
  ExecOptions,
  VMMetrics,
  HealthStatus,
  HatchClientOptions,
} from "./types.js";

export class HatchClient {
  private readonly apiUrl: string;
  private readonly apiKey: string;

  constructor(opts: HatchClientOptions) {
    this.apiUrl = (opts.apiUrl ?? "https://api.hatchvm.com").replace(/\/+$/, "");
    this.apiKey = opts.apiKey;
  }

  // ── VM lifecycle ──────────────────────────────────────────────

  async createVM(opts?: CreateVMOptions): Promise<VM> {
    return this.request<VM>("POST", "/vms", opts ?? {});
  }

  async listVMs(): Promise<VM[]> {
    return this.request<VM[]>("GET", "/vms");
  }

  async getVM(id: string): Promise<VM> {
    return this.request<VM>("GET", `/vms/${id}`);
  }

  async deleteVM(id: string): Promise<void> {
    await this.request("DELETE", `/vms/${id}`);
  }

  async stopVM(id: string): Promise<VM> {
    return this.request<VM>("POST", `/vms/${id}/stop`);
  }

  // ── Snapshots ─────────────────────────────────────────────────

  async snapshotVM(id: string): Promise<Snapshot> {
    return this.request<Snapshot>("POST", `/vms/${id}/snapshot`);
  }

  async restoreVM(id: string): Promise<VM> {
    return this.request<VM>("POST", `/vms/${id}/restore`);
  }

  async listSnapshots(id: string): Promise<Snapshot[]> {
    return this.request<Snapshot[]>("GET", `/vms/${id}/snapshots`);
  }

  // ── Routes ────────────────────────────────────────────────────

  async createRoute(vmId: string, opts: CreateRouteOptions): Promise<ProxyRoute> {
    return this.request<ProxyRoute>("POST", `/vms/${vmId}/routes`, opts);
  }

  async listRoutes(vmId: string): Promise<ProxyRoute[]> {
    return this.request<ProxyRoute[]>("GET", `/vms/${vmId}/routes`);
  }

  async deleteRoute(routeId: string): Promise<void> {
    await this.request("DELETE", `/routes/${routeId}`);
  }

  // ── Exec ──────────────────────────────────────────────────────

  async exec(vmId: string, command: string, opts?: ExecOptions): Promise<ExecResult> {
    return this.request<ExecResult>("POST", `/vms/${vmId}/exec`, {
      command,
      ...opts,
    });
  }

  // ── Metrics ───────────────────────────────────────────────────

  async getMetrics(vmId: string): Promise<VMMetrics> {
    return this.request<VMMetrics>("GET", `/vms/${vmId}/metrics`);
  }


  // ── Internal ──────────────────────────────────────────────────

  private async request<T = unknown>(
    method: string,
    path: string,
    body?: unknown,
  ): Promise<T> {
    const headers: Record<string, string> = {
      Authorization: `Bearer ${this.apiKey}`,
    };

    let reqBody: string | undefined;
    if (body !== undefined) {
      headers["Content-Type"] = "application/json";
      reqBody = JSON.stringify(body);
    }

    const res = await fetch(`${this.apiUrl}${path}`, {
      method,
      headers,
      body: reqBody,
    });

    if (!res.ok) {
      const text = await res.text();
      let message: string;
      try {
        const parsed = JSON.parse(text);
        message = parsed.error ?? text;
      } catch {
        message = text;
      }
      throw new HatchError(message, res.status);
    }

    const text = await res.text();
    if (!text.trim()) return undefined as T;
    return JSON.parse(text) as T;
  }
}

export class HatchError extends Error {
  constructor(
    message: string,
    public readonly status: number,
  ) {
    super(message);
    this.name = "HatchError";
  }
}
