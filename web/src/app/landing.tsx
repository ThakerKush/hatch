"use client";

import { useState } from "react";

import { authClient } from "@/lib/auth-client";

function AuthButton({ label, variant }: { label: string; variant: "outline" | "solid" }) {
  const [loading, setLoading] = useState(false);

  const base =
    "inline-flex items-center gap-2 px-4 py-1.5 text-[11px] tracking-[0.18em] uppercase font-mono transition-colors cursor-pointer disabled:opacity-50";
  const styles =
    variant === "solid"
      ? `${base} bg-zinc-100 text-black hover:bg-white border border-zinc-100`
      : `${base} bg-transparent text-zinc-400 hover:text-zinc-100 border border-zinc-700 hover:border-zinc-500`;

  return (
    <button
      type="button"
      disabled={loading}
      className={styles}
      onClick={async () => {
        setLoading(true);
        await authClient.signIn.social({ provider: "google", callbackURL: "/dashboard" });
        setLoading(false);
      }}
    >
      {loading ? "..." : label}
    </button>
  );
}

const FEATURES = [
  {
    cmd: "hatch vm create --vcpu 1 --mem 512",
    label: "Fast lifecycle",
    desc: "Sub-second microVM boot via Firecracker. Create, start, stop, and destroy in milliseconds — not minutes.",
    badge: "~80ms boot",
  },
  {
    cmd: "hatch vm snapshot <id>",
    label: "Snapshot / restore",
    desc: "Freeze a running VM's full memory and disk state. Restore it instantly later — agents resume exactly where they left off.",
    badge: "zero-copy restore",
  },
  {
    cmd: "hatch vm create --isolated",
    label: "Isolated runtime",
    desc: "Every agent gets a fresh, ephemeral microVM with its own kernel. No shared state, no noisy neighbours, complete workload isolation.",
    badge: "per-agent kernel",
  },
  {
    cmd: "curl https://<vm-id>.hatchvm.com",
    label: "Subdomain routing",
    desc: "Each VM gets its own subdomain. The reverse proxy routes requests to the right VM and auto-wakes snapshotted instances on the first hit.",
    badge: "auto-wake",
  },
];

type TerminalLine =
  | { type: "comment"; text: string }
  | { type: "command"; text: string }
  | { type: "output"; text: string }
  | { type: "blank" };

const QUICKSTART: TerminalLine[] = [
  { type: "comment", text: "# create a microVM" },
  { type: "command", text: "curl -X POST https://api.hatchvm.com/v1/vms \\" },
  { type: "output", text: '    -H "x-hatch-api-key: hatch_<key>" \\' },
  { type: "output", text: `    -d '{"vcpu_count":1,"mem_mib":512}'` },
  { type: "blank" },
  { type: "output", text: '{ "id": "vm-a1b2c3", "state": "running" }' },
  { type: "blank" },
  { type: "comment", text: "# snapshot it" },
  { type: "command", text: "curl -X POST https://api.hatchvm.com/v1/vms/vm-a1b2c3/snapshot \\" },
  { type: "output", text: '    -H "x-hatch-api-key: hatch_<key>"' },
  { type: "blank" },
  { type: "output", text: '{ "snapshot_id": "snap-x9y8z7", "ok": true }' },
  { type: "blank" },
  { type: "comment", text: "# hit it via its subdomain — auto-wakes if snapshotted" },
  { type: "command", text: "curl https://vm-a1b2c3.hatchvm.com/health" },
  { type: "blank" },
  { type: "output", text: '{ "status": "ok" }' },
];

function TerminalLine({ line }: { line: TerminalLine }) {
  if (line.type === "blank") return <div className="h-4" />;
  if (line.type === "comment")
    return <div className="text-xs leading-6 text-zinc-600">{line.text}</div>;
  if (line.type === "command")
    return (
      <div className="flex gap-2 text-xs leading-6">
        <span className="shrink-0 text-green-400">$</span>
        <span className="text-zinc-200">{line.text}</span>
      </div>
    );
  return <div className="text-xs leading-6 text-zinc-500">{line.text}</div>;
}

export function LandingPage() {
  return (
    <div className="min-h-screen bg-black font-mono text-zinc-300 selection:bg-zinc-700">
      {/* ── nav ─────────────────────────────────────────────────── */}
      <header className="sticky top-0 z-10 border-b border-zinc-900 bg-black/95 backdrop-blur">
        <div className="mx-auto flex max-w-5xl items-center justify-between px-6 py-4">
          <div className="flex items-center gap-3">
            <span className="text-zinc-200">▣</span>
            <span className="text-sm font-bold tracking-[0.3em] text-zinc-100">HATCH</span>
            <span className="hidden text-zinc-700 sm:inline">│</span>
            <span className="hidden text-[10px] tracking-[0.2em] text-zinc-500 sm:inline">
              MICRO VM ORCHESTRATION
            </span>
          </div>
          <div className="flex items-center gap-3">
            <AuthButton label="log in" variant="outline" />
            <AuthButton label="sign up" variant="solid" />
          </div>
        </div>
      </header>

      {/* ── hero ────────────────────────────────────────────────── */}
      <section className="mx-auto max-w-5xl px-6 pb-20 pt-20">
        <pre className="mb-8 text-[9px] leading-[1.4] text-zinc-600 sm:text-[11px]">{`  ██╗  ██╗ █████╗ ████████╗ ██████╗██╗  ██╗
  ██║  ██║██╔══██╗╚══██╔══╝██╔════╝██║  ██║
  ███████║███████║   ██║   ██║     ███████║
  ██╔══██║██╔══██║   ██║   ██║     ██╔══██║
  ██║  ██║██║  ██║   ██║   ╚██████╗██║  ██║
  ╚═╝  ╚═╝╚═╝  ╚═╝   ╚═╝    ╚═════╝╚═╝  ╚═╝`}</pre>

        <div className="mb-4 text-[11px] tracking-[0.2em] text-zinc-500">
          $ hatch --version 0.1.0-alpha
        </div>

        <h1 className="mb-3 max-w-xl text-xl leading-8 text-zinc-100">
          Lightweight microVM orchestration for AI agents.
        </h1>
        <p className="mb-10 max-w-xl text-sm leading-7 text-zinc-400">
          Spin up isolated Firecracker microVMs in milliseconds. Snapshot and restore agent state
          instantly. Route traffic to any VM via its own subdomain — with auto-wake on the first
          request.
        </p>

        <div className="flex flex-wrap items-center gap-4">
          <AuthButton label="get started →" variant="solid" />
          <a
            href="https://github.com/thakerkush/hatch"
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-2 text-[11px] uppercase tracking-[0.14em] text-zinc-500 transition-colors hover:text-zinc-300"
          >
            <span className="text-zinc-600">▸</span> view on github
          </a>
        </div>
      </section>

      {/* ── features ────────────────────────────────────────────── */}
      <section className="mx-auto max-w-5xl px-6 pb-24">
        <div className="mb-6 text-[10px] tracking-[0.22em] text-zinc-500">── FEATURES</div>

        <div className="grid gap-px border border-zinc-900 bg-zinc-900 sm:grid-cols-2">
          {FEATURES.map((f) => (
            <div key={f.label} className="flex flex-col gap-4 bg-black p-6">
              <div className="flex items-center gap-2">
                <span className="text-green-400 text-xs">$</span>
                <code className="overflow-x-auto whitespace-nowrap text-xs text-zinc-400">
                  {f.cmd}
                </code>
              </div>

              <div className="flex items-center gap-3">
                <span className="text-sm font-medium text-zinc-100">{f.label}</span>
                <span className="border border-zinc-800 px-2 py-0.5 text-[9px] tracking-[0.14em] uppercase text-zinc-500">
                  {f.badge}
                </span>
              </div>

              <p className="text-sm leading-6 text-zinc-400">{f.desc}</p>
            </div>
          ))}
        </div>
      </section>

      {/* ── quickstart ──────────────────────────────────────────── */}
      <section className="mx-auto max-w-5xl px-6 pb-24">
        <div className="mb-6 text-[10px] tracking-[0.22em] text-zinc-500">── QUICKSTART</div>

        <div className="border border-zinc-800 bg-zinc-950">
          {/* title bar */}
          <div className="flex items-center gap-2 border-b border-zinc-800 px-4 py-2.5">
            <span className="h-2.5 w-2.5 rounded-full bg-zinc-700" />
            <span className="h-2.5 w-2.5 rounded-full bg-zinc-700" />
            <span className="h-2.5 w-2.5 rounded-full bg-zinc-700" />
            <span className="ml-3 text-[10px] tracking-widest text-zinc-600">terminal</span>
          </div>
          <div className="p-5">
            {QUICKSTART.map((line, i) => (
              <TerminalLine key={i} line={line} />
            ))}
          </div>
        </div>
      </section>

      {/* ── cta ─────────────────────────────────────────────────── */}
      <section className="mx-auto max-w-5xl border-t border-zinc-900 px-6 py-20">
        <p className="mb-2 text-lg text-zinc-100">Ready to spin up your first VM?</p>
        <p className="mb-8 text-sm text-zinc-400">
          Free to start. No credit card required.
        </p>
        <AuthButton label="create free account →" variant="solid" />
      </section>

      {/* ── footer ──────────────────────────────────────────────── */}
      <footer className="border-t border-zinc-900 px-6 py-8">
        <div className="mx-auto flex max-w-5xl items-center justify-between text-[10px] tracking-[0.14em] text-zinc-600">
          <span>© 2026 hatch · hatchvm.com</span>
          <span>
            <span className="text-green-500">●</span> all systems nominal
          </span>
        </div>
      </footer>
    </div>
  );
}
