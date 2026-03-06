"use client";

import { Card } from "@/components/ui/card";

import type { DashboardVM, VMState } from "./types";
import { UsageBar } from "./usage-bar";

type VMTableProps = {
  vms: DashboardVM[];
};

function stateLabel(state: VMState) {
  switch (state) {
    case "running":
      return { glyph: "● RUN", color: "text-zinc-200" };
    case "starting":
      return { glyph: "◌ BOOT", color: "text-zinc-400" };
    case "stopping":
      return { glyph: "◌ STOP", color: "text-zinc-500" };
    case "snapshotted":
      return { glyph: "◌ SNAP", color: "text-zinc-400" };
    case "stopped":
      return { glyph: "○ OFF", color: "text-zinc-600" };
    default:
      return { glyph: "◍ ERR", color: "text-red-400" };
  }
}

export function VMTable({ vms }: VMTableProps) {
  return (
    <Card className="rounded-none border-zinc-800 bg-zinc-950">
      <div className="flex items-center justify-between border-b border-zinc-800 px-5 py-3 font-mono">
        <span className="text-[10px] tracking-[0.24em] text-zinc-500">VIRTUAL MACHINES</span>
        <span className="text-[11px] text-zinc-600">backed by hatchd API</span>
      </div>

      <div className="grid grid-cols-[minmax(120px,1.1fr)_100px_minmax(180px,1fr)_minmax(180px,1fr)_150px_100px_70px] gap-3 border-b border-zinc-900 px-5 py-2 font-mono text-[10px] tracking-[0.2em] text-zinc-500">
        <span>ID</span>
        <span>STATUS</span>
        <span>CPU</span>
        <span>MEM</span>
        <span>NETWORK</span>
        <span>UPTIME</span>
        <span>SNAPS</span>
      </div>

      {vms.length === 0 ? (
        <div className="px-5 py-7 font-mono text-sm text-zinc-500">No VMs found.</div>
      ) : (
        vms.map((vm) => {
          const status = stateLabel(vm.state);
          return (
            <div
              key={vm.id}
              className="grid grid-cols-[minmax(120px,1.1fr)_100px_minmax(180px,1fr)_minmax(180px,1fr)_150px_100px_70px] items-center gap-3 border-b border-zinc-900 px-5 py-3 font-mono text-xs last:border-b-0"
            >
              <span className="text-zinc-300">{vm.id}</span>
              <span className={status.color}>{status.glyph}</span>
              <UsageBar value={vm.cpuPercent} />
              <UsageBar value={vm.memPercent} />
              <span className="text-zinc-400">{vm.network}</span>
              <span className="text-zinc-400">{vm.uptimeLabel}</span>
              <span className="text-zinc-400">{vm.snapshots > 0 ? vm.snapshots : "—"}</span>
            </div>
          );
        })
      )}
    </Card>
  );
}
