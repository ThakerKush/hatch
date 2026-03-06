"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { Card } from "@/components/ui/card";
import { authClient } from "@/lib/auth-client";

import { ApiKeyList } from "./api-key-list";
import { CodeSnippets } from "./code-snippets";
import { CreateKeyModal } from "./create-key-modal";
import { DashboardNavbar } from "./navbar";
import { StatCard } from "./stat-card";
import type {
  DashboardApiKey,
  DashboardStats,
  DashboardVM,
  NavTab,
  SnapshotRecord,
  VMMetrics,
  VMRecord,
} from "./types";
import { VMTable } from "./vm-table";

type DashboardShellProps = {
  userLabel: string;
};

type NormalizedCreateKeyResult = {
  id: string;
  plainTextKey: string;
};

function formatUptime(seconds: number): string {
  if (seconds <= 0) {
    return "—";
  }
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (days > 0) {
    return `${days}d ${hours}h`;
  }
  if (hours > 0) {
    return `${hours}h ${minutes}m`;
  }
  return `${minutes}m`;
}

function formatMBPerSecond(bytes: number): string {
  const mb = bytes / (1024 * 1024);
  return mb.toFixed(1);
}

function formatRelativeDate(value: string): string {
  if (!value) {
    return "unknown";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "unknown";
  }
  return date.toISOString().slice(0, 10);
}

function getApiKeyClient() {
  const client = authClient as unknown as {
    apiKey?: {
      create?: (input: Record<string, unknown>) => Promise<unknown>;
      list?: (input?: Record<string, unknown>) => Promise<unknown>;
      delete?: (input: Record<string, unknown>) => Promise<unknown>;
    };
  };
  return client.apiKey;
}

async function createDashboardApiKey(name: string): Promise<NormalizedCreateKeyResult> {
  const apiKey = getApiKeyClient();
  if (!apiKey) {
    throw new Error("API key client unavailable");
  }

  let result: unknown;
  if (apiKey.create) {
    result = await apiKey.create({ name });
  } else {
    throw new Error("API key create method is unavailable");
  }

  const payload = result as Record<string, unknown>;
  const inner = (payload?.data ?? payload) as Record<string, unknown>;
  const plainTextKey = (inner?.plainTextKey || inner?.key || "") as string;
  const id = (inner?.id || "") as string;
  if (!plainTextKey || !id) {
    throw new Error("Could not read the newly created API key");
  }

  return { id, plainTextKey };
}

async function listDashboardApiKeys(): Promise<DashboardApiKey[]> {
  const apiKey = getApiKeyClient();
  if (!apiKey) {
    return [];
  }

  let result: unknown;
  if (apiKey.list) {
    result = await apiKey.list();
  } else {
    return [];
  }

  const payload = result as Record<string, unknown>;
  const inner = (payload?.data ?? payload) as Record<string, unknown>;
  const raw = inner?.apiKeys ?? inner?.data ?? inner;
  const entries = Array.isArray(raw) ? raw : [];

  return entries.map((entry, index) => {
    const row = entry as {
      id?: string;
      name?: string;
      start?: string;
      prefix?: string;
      createdAt?: string;
      lastRequest?: string;
    };
    const prefix = [row.prefix, row.start].filter(Boolean).join("") || "hatch_****";
    return {
      id: row.id || `key-${index}`,
      name: row.name || "unnamed-key",
      prefix: `${prefix}...`,
      createdAt: formatRelativeDate(row.createdAt || ""),
      lastUsed: row.lastRequest ? formatRelativeDate(row.lastRequest) : "never",
    };
  });
}

async function revokeDashboardApiKey(keyId: string): Promise<void> {
  const apiKey = getApiKeyClient();
  if (!apiKey) {
    return;
  }

  if (apiKey.delete) {
    await apiKey.delete({ keyId });
  }
}

export function DashboardShell({ userLabel }: DashboardShellProps) {
  const [nav, setNav] = useState<NavTab>("dashboard");
  const [vms, setVMs] = useState<DashboardVM[]>([]);
  const [keys, setKeys] = useState<DashboardApiKey[]>([]);
  const [loadingVMs, setLoadingVMs] = useState(true);
  const [loadingKeys, setLoadingKeys] = useState(true);
  const [error, setError] = useState("");
  const [modalOpen, setModalOpen] = useState(false);
  const initialLoadDone = useRef(false);

  const loadKeys = useCallback(async () => {
    setLoadingKeys(true);
    try {
      const list = await listDashboardApiKeys();
      setKeys(list);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to load API keys");
    } finally {
      setLoadingKeys(false);
    }
  }, []);

  const loadVMs = useCallback(async () => {
    if (!initialLoadDone.current) {
      setLoadingVMs(true);
    }
    setError("");
    try {
      const vmsResponse = await fetch("/api/v1/vms", {
        credentials: "include",
        cache: "no-store",
      });
      if (!vmsResponse.ok) {
        const payload = (await vmsResponse.json().catch(() => ({}))) as { error?: string };
        throw new Error(payload.error || "Unable to load VMs");
      }

      const vmRecords = ((await vmsResponse.json()) as VMRecord[] | null) ?? [];
      const enriched = await Promise.all(
        vmRecords.map(async (vm): Promise<DashboardVM> => {
          let metrics: VMMetrics | null = null;
          let snapshots: SnapshotRecord[] = [];

          try {
            const [metricsRes, snapshotsRes] = await Promise.all([
              fetch(`/api/v1/vms/${vm.id}/metrics`, {
                credentials: "include",
                cache: "no-store",
              }),
              fetch(`/api/v1/vms/${vm.id}/snapshots`, {
                credentials: "include",
                cache: "no-store",
              }),
            ]);

            if (metricsRes.ok) {
              metrics = ((await metricsRes.json()) as VMMetrics | null) ?? null;
            }
            if (snapshotsRes.ok) {
              snapshots = ((await snapshotsRes.json()) as SnapshotRecord[] | null) ?? [];
            }
          } catch {
            // enrichment failed for this VM — continue with defaults
          }

          const memPercent = Math.max(5, Math.min(100, Math.round((vm.mem_mib / 4096) * 100)));
          const cpuPercent = Math.round(metrics?.vcpu?.utilization_percent ?? 0);
          const tx = formatMBPerSecond(metrics?.net?.tx_bytes ?? 0);
          const rx = formatMBPerSecond(metrics?.net?.rx_bytes ?? 0);
          return {
            id: vm.id,
            state: vm.state,
            vcpu: vm.vcpu_count,
            memMib: vm.mem_mib,
            cpuPercent,
            memPercent,
            network: `↑ ${tx} ↓ ${rx}`,
            uptimeLabel: formatUptime(metrics?.uptime_seconds ?? 0),
            snapshots: snapshots.length,
          };
        }),
      );
      setVMs(enriched);
    } catch (err) {
      if (!initialLoadDone.current) {
        setError(err instanceof Error ? err.message : "Unable to load dashboard");
      }
    } finally {
      initialLoadDone.current = true;
      setLoadingVMs(false);
    }
  }, []);

  useEffect(() => {
    void loadKeys();
    void loadVMs();
  }, [loadKeys, loadVMs]);

  useEffect(() => {
    const interval = setInterval(() => void loadVMs(), 10_000);
    return () => clearInterval(interval);
  }, [loadVMs]);

  const stats: DashboardStats = useMemo(() => {
    const active = vms.filter((vm) => vm.state === "running").length;
    const snapshots = vms.reduce((sum, vm) => sum + vm.snapshots, 0);
    return {
      activeVMs: active,
      totalVMs: vms.length,
      snapshots,
    };
  }, [vms]);

  const onCreateKey = useCallback(
    async (name: string) => {
      const created = await createDashboardApiKey(name);
      await loadKeys();
      return { plainTextKey: created.plainTextKey };
    },
    [loadKeys],
  );

  const onRevoke = useCallback(
    async (keyId: string) => {
      await revokeDashboardApiKey(keyId);
      await loadKeys();
    },
    [loadKeys],
  );

  return (
    <div className="min-h-screen bg-black text-zinc-200">
      <DashboardNavbar activeTab={nav} onTabChange={setNav} userLabel={userLabel} />

      <main className="mx-auto max-w-6xl px-8 py-10 font-mono">
        <pre className="mb-2 text-xs leading-5 text-zinc-600">{`┌──────────────────────────────────────────────────────┐
│  h a t c h   / /   c o n t r o l   p l a n e         │
└──────────────────────────────────────────────────────┘`}</pre>
        <p className="mb-8 text-xs tracking-[0.1em] text-zinc-500">
          {new Date().toUTCString()} · {stats.totalVMs} vms registered · {stats.activeVMs} running
        </p>

        {error ? (
          <Card className="mb-6 rounded-none border-red-900 bg-zinc-950 p-4 text-sm text-red-300">{error}</Card>
        ) : null}

        <section className="mb-4 grid grid-cols-1 gap-2 md:grid-cols-2">
          <StatCard
            label="VMs Active"
            value={`${stats.activeVMs} / ${stats.totalVMs}`}
            subText={`${stats.totalVMs - stats.activeVMs} not running`}
          />
          <StatCard label="Snapshots" value={`${stats.snapshots}`} subText="across all VMs" />
        </section>

        <section className="mb-4">
          {loadingVMs ? (
            <Card className="rounded-none border-zinc-800 bg-zinc-950 p-5 text-zinc-500">
              Loading VM data...
            </Card>
          ) : (
            <VMTable vms={vms} />
          )}
        </section>

        <section className="mb-4 grid grid-cols-1 gap-2 lg:grid-cols-2">
          <CodeSnippets />

          <ApiKeyList
            keys={keys}
            loading={loadingKeys}
            onCreateClick={() => setModalOpen(true)}
            onRevoke={(keyId) => void onRevoke(keyId)}
          />
        </section>
      </main>

      <CreateKeyModal open={modalOpen} onOpenChange={setModalOpen} onCreateKey={onCreateKey} />
    </div>
  );
}
