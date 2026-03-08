"use client";

import { useState, useEffect } from "react";

import { authClient } from "@/lib/auth-client";

function AuthButton({ label, variant }: { label: string; variant: "outline" | "solid" }) {
  const [loading, setLoading] = useState(false);

  const base =
    "inline-flex items-center gap-2 px-4 py-1.5 text-[11px] tracking-[0.18em] uppercase font-mono transition-all duration-300 cursor-pointer disabled:opacity-50";
  const styles =
    variant === "solid"
      ? `${base} bg-zinc-100 text-black hover:bg-white border border-zinc-100 hover:shadow-[0_0_20px_rgba(255,255,255,0.15)]`
      : `${base} bg-transparent text-zinc-400 hover:text-zinc-100 border border-zinc-700 hover:border-zinc-500 hover:shadow-[0_0_15px_rgba(255,255,255,0.05)]`;

  return (
    <button
      type="button"
      disabled={loading}
      className={styles}
      onClick={async () => {
        setLoading(true);
        await authClient.signIn.social({
          provider: "google",
          callbackURL: "/dashboard",
        });
        setLoading(false);
      }}
    >
      {loading ? "..." : label}
    </button>
  );
}

const FEATURES = [
  {
    cmd: "curl -X POST api.hatchvm.com/vms",
    label: "Fast lifecycle",
    desc: "Lightweight microVM boot via Firecracker. Create, stop, and destroy VMs with a single API call.",
    badge: "firecracker",
  },
  {
    cmd: "curl -X POST .../vms/{id}/snapshot",
    label: "Snapshot / restore",
    desc: "Freeze a running VM's full memory and disk state. Restore it later — agents resume exactly where they left off.",
    badge: "s3-backed",
  },
  {
    cmd: "curl -X POST .../vms -d '{\"user_data\":\"...\"}'",
    label: "Isolated runtime",
    desc: "Every agent gets a fresh, ephemeral microVM with its own kernel. Pass cloud-init user data to provision packages, users, and SSH keys.",
    badge: "per-agent kernel",
  },
  {
    cmd: "curl -X POST .../vms/{id}/routes",
    label: "Subdomain routing",
    desc: "Map a custom subdomain to any port on your VM. The reverse proxy routes traffic and auto-wakes snapshotted instances on the first hit.",
    badge: "auto-wake",
  },
];

type TerminalLine =
  | { type: "comment"; text: string }
  | { type: "command"; text: string }
  | { type: "output"; text: string }
  | { type: "blank" };

const QUICKSTART: TerminalLine[] = [
  { type: "comment", text: "# create a microVM with cloud-init user data" },
  { type: "command", text: "curl -X POST https://api.hatchvm.com/vms \\" },
  { type: "output", text: '    -H "Authorization: Bearer $HATCH_API_KEY" \\' },
  { type: "output", text: '    -H "Content-Type: application/json" \\' },
  { type: "output", text: `    -d '{"vcpu_count":2,"mem_mib":1024,"user_data":"#cloud-config\\n..."}'` },
  { type: "blank" },
  { type: "output", text: '{ "id": "vm-a1b2c3", "state": "running" }' },
  { type: "blank" },
  { type: "comment", text: "# snapshot it" },
  { type: "command", text: "curl -X POST https://api.hatchvm.com/vms/vm-a1b2c3/snapshot \\" },
  { type: "output", text: '    -H "Authorization: Bearer $HATCH_API_KEY"' },
  { type: "blank" },
  { type: "output", text: '{ "id": "snap-x9y8z7", "vm_id": "vm-a1b2c3" }' },
  { type: "blank" },
  { type: "comment", text: "# hit it via its subdomain — auto-wakes if snapshotted" },
  { type: "command", text: "curl https://hatchvm.com/healthz" },
  { type: "blank" },
  { type: "output", text: '{ "status": "ok" }' },
];

function TerminalLineComponent({ line }: { line: TerminalLine }) {
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

function BlinkingCursor() {
  return (
    <div className="flex gap-2 text-xs leading-6 mt-1">
      <span className="shrink-0 text-green-400">$</span>
      <span className="inline-block w-[7px] h-[14px] bg-green-400/80 animate-[blink_1s_steps(1)_infinite]" />
    </div>
  );
}

function FeatureCard({ f, index }: { f: typeof FEATURES[number]; index: number }) {
  return (
    <div
      className="group relative flex flex-col gap-4 bg-black p-6 transition-all duration-500 hover:bg-zinc-950"
      style={{ animationDelay: `${index * 100}ms` }}
    >
      {/* hover glow */}
      <div className="pointer-events-none absolute inset-0 opacity-0 transition-opacity duration-500 group-hover:opacity-100 bg-[radial-gradient(ellipse_at_center,rgba(74,222,128,0.04),transparent_70%)]" />

      <div className="relative flex items-center gap-2">
        <span className="text-green-400 text-xs transition-all duration-300 group-hover:text-green-300 group-hover:drop-shadow-[0_0_6px_rgba(74,222,128,0.5)]">$</span>
        <code className="overflow-x-auto whitespace-nowrap text-xs text-zinc-400 transition-colors duration-300 group-hover:text-zinc-300">
          {f.cmd}
        </code>
      </div>

      <div className="relative flex items-center gap-3">
        <span className="text-sm font-medium text-zinc-100">{f.label}</span>
        <span className="border border-zinc-800 px-2 py-0.5 text-[9px] tracking-[0.14em] uppercase text-zinc-500 transition-all duration-300 group-hover:border-green-900/50 group-hover:text-green-400/60">
          {f.badge}
        </span>
      </div>

      <p className="relative text-sm leading-6 text-zinc-400">{f.desc}</p>
    </div>
  );
}

function StatusIndicator() {
  const [visible, setVisible] = useState(true);

  useEffect(() => {
    const interval = setInterval(() => setVisible((v) => !v), 2000);
    return () => clearInterval(interval);
  }, []);

  return (
    <span className={`text-green-500 transition-opacity duration-1000 ${visible ? "opacity-100" : "opacity-40"}`}>
      ●
    </span>
  );
}

export function LandingPage() {
  return (
    <div className="relative min-h-screen bg-black font-mono text-zinc-300 selection:bg-green-900/40 selection:text-green-200">
      {/* ── background grid ───────────────────────────────────── */}
      <div
        className="pointer-events-none fixed inset-0 opacity-[0.03]"
        style={{
          backgroundImage: `
            linear-gradient(rgba(255,255,255,0.1) 1px, transparent 1px),
            linear-gradient(90deg, rgba(255,255,255,0.1) 1px, transparent 1px)
          `,
          backgroundSize: "60px 60px",
        }}
      />

      {/* ── scan lines ────────────────────────────────────────── */}
      <div
        className="pointer-events-none fixed inset-0 opacity-[0.015]"
        style={{
          backgroundImage: "repeating-linear-gradient(0deg, transparent, transparent 2px, rgba(255,255,255,0.1) 2px, rgba(255,255,255,0.1) 4px)",
        }}
      />

      {/* ── nav ─────────────────────────────────────────────────── */}
      <header className="sticky top-0 z-10 border-b border-zinc-900 bg-black/90 backdrop-blur-md">
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
      <section className="relative mx-auto max-w-5xl px-6 pb-20 pt-20">
        {/* glow behind ASCII art */}
        <div className="pointer-events-none absolute left-1/2 top-16 -translate-x-1/2 h-40 w-[500px] bg-green-500/[0.04] blur-[80px] rounded-full" />

        <pre className="relative mb-8 text-[9px] leading-[1.4] text-zinc-600 sm:text-[11px] drop-shadow-[0_0_30px_rgba(74,222,128,0.06)]">{`  ██╗  ██╗ █████╗ ████████╗ ██████╗██╗  ██╗
  ██║  ██║██╔══██╗╚══██╔══╝██╔════╝██║  ██║
  ███████║███████║   ██║   ██║     ███████║
  ██╔══██║██╔══██║   ██║   ██║     ██╔══██║
  ██║  ██║██║  ██║   ██║   ╚██████╗██║  ██║
  ╚═╝  ╚═╝╚═╝  ╚═╝   ╚═╝    ╚═════╝╚═╝  ╚═╝`}</pre>

        <div className="mb-4 text-[11px] tracking-[0.2em] text-zinc-500">
          v0.1.0-alpha · api-first · firecracker-backed
        </div>

        <h1 className="mb-3 max-w-xl text-xl leading-8 text-zinc-100">
          Lightweight microVM orchestration for AI agents.
        </h1>
        <p className="mb-10 max-w-xl text-sm leading-7 text-zinc-400">
          Spin up isolated Firecracker microVMs. Snapshot and restore agent state. Route traffic to any VM via its own subdomain — with auto-wake on the first
          request.
        </p>

        <div className="flex flex-wrap items-center gap-4">
          <AuthButton label="get started →" variant="solid" />
          <a
            href="https://github.com/thakerkush/hatch"
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-2 text-[11px] uppercase tracking-[0.14em] text-zinc-500 transition-colors duration-300 hover:text-zinc-300"
          >
            <span className="text-zinc-600 transition-colors duration-300 hover:text-green-400">▸</span> view on github
          </a>
        </div>
      </section>

      {/* ── features ────────────────────────────────────────────── */}
      <section className="mx-auto max-w-5xl px-6 pb-24">
        <div className="mb-6 text-[10px] tracking-[0.22em] text-zinc-500">── FEATURES</div>

        <div className="grid gap-px border border-zinc-900 bg-zinc-900 sm:grid-cols-2">
          {FEATURES.map((f, i) => (
            <FeatureCard key={f.label} f={f} index={i} />
          ))}
        </div>
      </section>

      {/* ── quickstart ──────────────────────────────────────────── */}
      <section className="mx-auto max-w-5xl px-6 pb-24">
        <div className="mb-6 text-[10px] tracking-[0.22em] text-zinc-500">── QUICKSTART</div>

        <div className="group/terminal relative border border-zinc-800 bg-zinc-950 transition-all duration-500 hover:border-zinc-700 hover:shadow-[0_0_40px_rgba(0,0,0,0.5)]">
          {/* title bar */}
          <div className="flex items-center gap-2 border-b border-zinc-800 px-4 py-2.5">
            <span className="h-2.5 w-2.5 rounded-full bg-zinc-700 transition-colors duration-300 group-hover/terminal:bg-red-500/70" />
            <span className="h-2.5 w-2.5 rounded-full bg-zinc-700 transition-colors duration-300 group-hover/terminal:bg-yellow-500/70" />
            <span className="h-2.5 w-2.5 rounded-full bg-zinc-700 transition-colors duration-300 group-hover/terminal:bg-green-500/70" />
            <span className="ml-3 text-[10px] tracking-widest text-zinc-600">terminal</span>
          </div>
          <div className="p-5">
            {QUICKSTART.map((line, i) => (
              <TerminalLineComponent key={i} line={line} />
            ))}
            <BlinkingCursor />
          </div>
        </div>
      </section>

      {/* ── cta ─────────────────────────────────────────────────── */}
      <section className="relative mx-auto max-w-5xl border-t border-zinc-900 px-6 py-20">
        <div className="pointer-events-none absolute inset-0 bg-gradient-to-b from-transparent via-green-500/[0.01] to-transparent" />
        <p className="relative mb-2 text-lg text-zinc-100">Ready to spin up your first VM?</p>
        <p className="relative mb-8 text-sm text-zinc-400">
          Free to start.
        </p>
        <div className="relative">
          <AuthButton label="create free account →" variant="solid" />
        </div>
      </section>

      {/* ── footer ──────────────────────────────────────────────── */}
      <footer className="border-t border-zinc-900 px-6 py-8">
        <div className="mx-auto flex max-w-5xl items-center justify-between text-[10px] tracking-[0.14em] text-zinc-600">
          <span>© 2026 hatch · hatchvm.com</span>
          <span>
            <StatusIndicator /> all systems nominal
          </span>
        </div>
      </footer>

    </div>
  );
}
