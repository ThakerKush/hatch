"use client";
import { useState } from "react";

const MONO = { fontFamily: "'Courier New', Courier, monospace" };
const SCALE = 1.25;
const fs = (size: number) => Math.round(size * SCALE);

type VMStatus = "running" | "snapshotting" | "stopped";
type HttpMethod = "GET" | "POST" | "DELETE" | "PUT";
type NavTab = "dashboard" | "keys" | "docs";

type VM = {
  id: string;
  status: VMStatus;
  cpu: number;
  mem: number;
  net: string;
  uptime: string;
  region: string;
  snaps: number;
};

type LogEntry = {
  ts: string;
  method: HttpMethod;
  path: string;
  status: number;
  ms: number;
};

type ApiKey = {
  name: string;
  key: string;
  created: string;
  last: string;
};

const VMS: VM[] = [
  { id: "vm-a3f2", status: "running",      cpu: 34, mem: 61, net: "↑ 1.2  ↓ 0.4", uptime: "14h 33m", region: "us-east-1",  snaps: 3 },
  { id: "vm-b91c", status: "running",      cpu: 78, mem: 88, net: "↑ 4.1  ↓ 2.0", uptime: "2h 07m",  region: "eu-west-2",  snaps: 1 },
  { id: "vm-c004", status: "snapshotting", cpu: 12, mem: 40, net: "↑ 0.1  ↓ 0.0", uptime: "6d 11h",  region: "us-west-2",  snaps: 7 },
  { id: "vm-d7a0", status: "stopped",      cpu: 0,  mem: 0,  net: "—",             uptime: "—",        region: "ap-south-1", snaps: 2 },
];

const LOGS: LogEntry[] = [
  { ts: "03:41:02", method: "POST",   path: "/v1/vms",               status: 201, ms: 84  },
  { ts: "03:40:58", method: "GET",    path: "/v1/vms/vm-b91c",       status: 200, ms: 12  },
  { ts: "03:40:44", method: "POST",   path: "/v1/vms/vm-c004/snap",  status: 202, ms: 310 },
  { ts: "03:40:21", method: "DELETE", path: "/v1/vms/vm-d7a0",       status: 204, ms: 55  },
  { ts: "03:39:55", method: "GET",    path: "/v1/vms",               status: 200, ms: 9   },
];

const SNIPPETS = {
  create: `curl -X POST https://api.hatch.run/v1/vms \\
  -H "Authorization: Bearer $HATCH_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "vcpus": 2,
    "mem_mb": 1024,
    "image": "ubuntu-22.04",
    "region": "us-east-1"
  }'`,
  ssh: `# Returns a one-time SSH token bound to the VM session
curl -X POST https://api.hatch.run/v1/vms/vm-a3f2/ssh \\
  -H "Authorization: Bearer $HATCH_KEY"

# Use the returned ephemeral host + port to connect
ssh -p 2222 root@ssh.hatch.run -i ~/.ssh/id_ed25519`,
  snapshot: `curl -X POST https://api.hatch.run/v1/vms/vm-a3f2/snapshots \\
  -H "Authorization: Bearer $HATCH_KEY" \\
  -d '{ "label": "pre-deploy-v2" }'`,
  restore: `curl -X POST https://api.hatch.run/v1/vms/vm-a3f2/restore \\
  -H "Authorization: Bearer $HATCH_KEY" \\
  -d '{ "snapshot_id": "snap_9x2k1p" }'`,
};

const INIT_KEYS: ApiKey[] = [
  { name: "prod-backend", key: "htk_live_9xK2…mP3q", created: "2025-11-03", last: "2m ago"  },
  { name: "staging",      key: "htk_live_4rT7…aZ1v", created: "2025-12-14", last: "1h ago"  },
  { name: "local-dev",    key: "htk_test_0bW9…cL8x", created: "2026-01-22", last: "3d ago"  },
];

const statusStyle = (s: VMStatus) => ({
  running:      { color: "#e0e0e0", glyph: "● RUN"  },
  snapshotting: { color: "#a1a1a1", glyph: "◌ SNAP" },
  stopped:      { color: "#7a7a7a", glyph: "○ OFF"  },
}[s] || { color: "#8a8a8a", glyph: s });

const methodColor = (m: HttpMethod) => ({
  GET: "#d8d8d8", POST: "#efefef", DELETE: "#9c9c9c", PUT: "#bcbcbc",
}[m] || "#9a9a9a");

const Bar = ({ value }: { value: number }) => {
  const filled = Math.round(value / 5);
  const dimmed = value === 0;
  const hi = value >= 90;
  const mid = value >= 70;
  const barColor = dimmed ? "#474747" : hi ? "#b8b8b8" : mid ? "#a0a0a0" : "#878787";
  return (
    <span style={{ letterSpacing: 0.5 }}>
      <span style={{ color: barColor }}>{"█".repeat(filled)}</span>
      <span style={{ color: "#3c3c3c" }}>{"░".repeat(20 - filled)}</span>
      <span style={{ color: dimmed ? "#626262" : "#a0a0a0", marginLeft: 6 }}>{value}%</span>
    </span>
  );
};

export default function Hatch() {
  const [nav, setNav]         = useState<NavTab>("dashboard");
  const [snippet, setSnippet] = useState<keyof typeof SNIPPETS>("create");
  const [keys, setKeys]       = useState(INIT_KEYS);
  const [creating, setCreating] = useState(false);
  const [newName, setNewName]   = useState("");
  const snippetKeys = Object.keys(SNIPPETS) as Array<keyof typeof SNIPPETS>;

  const addKey = () => {
    if (!newName.trim()) return;
    const rand = () => Math.random().toString(36).slice(2, 6);
    setKeys([...keys, { name: newName.trim(), key: `htk_live_${rand()}…${rand()}`, created: "2026-03-04", last: "just now" }]);
    setNewName(""); setCreating(false);
  };

  const divider = "1px solid #262626";

  return (
    <div style={{ ...MONO, background: "#0a0a0a", minHeight: "100vh", color: "#b0b0b0", fontSize: fs(12), lineHeight: 1.45 }}>
      <div style={{
        position: "sticky", top: 0, zIndex: 99,
        background: "rgba(10,10,10,0.9)", backdropFilter: "blur(14px)",
        borderBottom: divider,
        display: "flex", alignItems: "center", justifyContent: "space-between",
        padding: "14px 36px",
      }}>
        <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
          <pre style={{ margin: 0, color: "#ededed", fontSize: fs(13), lineHeight: 1 }}>{`▣`}</pre>
          <span style={{ color: "#f0f0f0", fontWeight: "bold", letterSpacing: 3, fontSize: fs(13) }}>HATCH</span>
          <span style={{ color: "#686868", margin: "0 4px" }}>│</span>
          <span style={{ color: "#959595", fontSize: fs(10), letterSpacing: 2 }}>MICRO VM ORCHESTRATION</span>
        </div>
        <div style={{ display: "flex", gap: 32 }}>
          {(["dashboard", "keys", "docs"] as NavTab[]).map(n => (
            <span key={n} onClick={() => setNav(n)} style={{
              cursor: "pointer", fontSize: fs(10), letterSpacing: 2,
              textTransform: "uppercase",
              color: nav === n ? "#e0e0e0" : "#8a8a8a",
              borderBottom: nav === n ? "1px solid #666" : "1px solid transparent",
              paddingBottom: 3, transition: "color 0.15s",
            }}>{n}</span>
          ))}
        </div>
        <div style={{ fontSize: fs(10), color: "#8a8a8a", letterSpacing: 1 }}>
          <span style={{ color: "#9a9a9a" }}>●</span> &nbsp;acme-corp &nbsp;·&nbsp; api v1
        </div>
      </div>

      <div style={{ maxWidth: 1080, margin: "0 auto", padding: "44px 36px" }}>
        <pre style={{ margin: "0 0 6px 0", color: "#6e6e6e", fontSize: fs(11), lineHeight: 1.3 }}>{
`┌──────────────────────────────────────────────────────┐
│  h a t c h   / /   c o n t r o l   p l a n e         │
└──────────────────────────────────────────────────────┘`}
        </pre>
        <div style={{ color: "#8c8c8c", fontSize: fs(11), marginBottom: 40, letterSpacing: 1 }}>
          UTC&nbsp;03:41:09 &nbsp;·&nbsp; 4 vms registered &nbsp;·&nbsp; 3 running
        </div>

        <div style={{ display: "grid", gridTemplateColumns: "repeat(4,1fr)", gap: 1, marginBottom: 1 }}>
          {[
            { label: "VMs Active",      value: "3 / 4",  sub: "1 stopped"       },
            { label: "Snapshots",        value: "13",     sub: "across all vms"  },
            { label: "API Calls Today",  value: "1,847",  sub: "↑ 12% vs yesterday" },
            { label: "Avg Latency",      value: "94ms",   sub: "last 100 requests"  },
          ].map(({ label, value, sub }) => (
            <div key={label} style={{ background: "#111111", border: divider, padding: "22px 24px" }}>
              <div style={{ color: "#919191", fontSize: fs(9), letterSpacing: 3, marginBottom: 10 }}>{label.toUpperCase()}</div>
              <div style={{ color: "#ececec", fontSize: fs(28), marginBottom: 6, fontWeight: 100 }}>{value}</div>
              <div style={{ color: "#9a9a9a", fontSize: fs(10) }}>{sub}</div>
            </div>
          ))}
        </div>

        <div style={{ background: "#111111", border: divider, marginBottom: 1 }}>
          <div style={{ padding: "13px 22px", borderBottom: divider, display: "flex", justifyContent: "space-between", alignItems: "center" }}>
            <span style={{ color: "#959595", fontSize: fs(9), letterSpacing: 3 }}>VIRTUAL MACHINES</span>
            <span style={{ color: "#757575", fontSize: fs(10) }}>read-only · manage via api</span>
          </div>
          <div style={{
            display: "grid", gridTemplateColumns: "110px 80px 1fr 1fr 150px 90px 60px",
            padding: "9px 22px", color: "#8d8d8d", fontSize: fs(9), letterSpacing: 2,
            borderBottom: "1px solid #111",
          }}>
            {["ID","STATUS","CPU","MEM","NETWORK (MB/S)","UPTIME","SNAPS"].map(h => <span key={h}>{h}</span>)}
          </div>
          {VMS.map((vm, i) => {
            const { color, glyph } = statusStyle(vm.status);
            return (
              <div key={vm.id} style={{
                display: "grid", gridTemplateColumns: "110px 80px 1fr 1fr 150px 90px 60px",
                padding: "13px 22px", alignItems: "center",
                borderBottom: i < VMS.length - 1 ? "1px solid #0e0e0e" : "none",
              }}>
                <span style={{ color: "#ababab" }}>{vm.id}</span>
                <span style={{ color, fontSize: fs(10) }}>{glyph}</span>
                <Bar value={vm.cpu} />
                <Bar value={vm.mem} />
                <span style={{ color: "#969696", fontSize: fs(11) }}>{vm.net}</span>
                <span style={{ color: "#9f9f9f", fontSize: fs(11) }}>{vm.uptime}</span>
                <span style={{ color: vm.snaps > 0 ? "#9f9f9f" : "#6a6a6a", fontSize: fs(11) }}>{vm.snaps > 0 ? vm.snaps : "—"}</span>
              </div>
            );
          })}
        </div>

        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1, marginBottom: 1 }}>
          <div style={{ background: "#111111", border: divider }}>
            <div style={{ padding: "13px 22px", borderBottom: divider }}>
              <span style={{ color: "#959595", fontSize: fs(9), letterSpacing: 3 }}>RECENT API REQUESTS</span>
            </div>
            {LOGS.map((l, i) => (
              <div key={i} style={{
                display: "grid", gridTemplateColumns: "62px 50px 1fr 36px 44px",
                padding: "11px 22px", gap: 8, alignItems: "center", fontSize: fs(11),
                borderBottom: i < LOGS.length - 1 ? "1px solid #0e0e0e" : "none",
              }}>
                <span style={{ color: "#808080" }}>{l.ts}</span>
                <span style={{ color: methodColor(l.method) }}>{l.method}</span>
                <span style={{ color: "#9a9a9a", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{l.path}</span>
                <span style={{ color: l.status < 300 ? "#b2b2b2" : "#8a8a8a" }}>{l.status}</span>
                <span style={{ color: "#7f7f7f" }}>{l.ms}ms</span>
              </div>
            ))}
          </div>

          <div style={{ background: "#111111", border: divider }}>
            <div style={{ padding: "13px 22px", borderBottom: divider, display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <span style={{ color: "#959595", fontSize: fs(9), letterSpacing: 3 }}>API KEYS</span>
              <span onClick={() => setCreating(!creating)} style={{ color: "#c6c6c6", fontSize: fs(10), cursor: "pointer", letterSpacing: 1 }}>
                {creating ? "✕ cancel" : "+ new key"}
              </span>
            </div>
            {creating && (
              <div style={{ padding: "12px 22px", borderBottom: divider, display: "flex", gap: 8 }}>
                <input
                  autoFocus
                  value={newName}
                  onChange={e => setNewName(e.target.value)}
                  onKeyDown={e => e.key === "Enter" && addKey()}
                  placeholder="key name (e.g. prod-worker)"
                  style={{
                    ...MONO, flex: 1, background: "#000", border: "1px solid #222",
                    color: "#c0c0c0", padding: "7px 10px", fontSize: fs(11), outline: "none",
                  }}
                />
                <button onClick={addKey} style={{
                  ...MONO, background: "transparent", border: "1px solid #333",
                  color: "#c0c0c0", padding: "7px 16px", cursor: "pointer", fontSize: fs(11),
                }}>create</button>
              </div>
            )}
            {keys.map((k, i) => (
              <div key={i} style={{ padding: "13px 22px", borderBottom: i < keys.length - 1 ? "1px solid #0e0e0e" : "none" }}>
                <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 5 }}>
                  <span style={{ color: "#c0c0c0" }}>{k.name}</span>
                  <span style={{ color: "#7a7a7a", fontSize: fs(10) }}>used {k.last}</span>
                </div>
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                  <span style={{ color: "#9c9c9c", fontSize: fs(11) }}>{k.key}</span>
                  <span style={{ color: "#7a7a7a", fontSize: fs(10), cursor: "pointer" }}>revoke</span>
                </div>
              </div>
            ))}
          </div>
        </div>

        <div style={{ background: "#111111", border: divider }}>
          <div style={{ borderBottom: divider, display: "flex" }}>
            {snippetKeys.map(k => (
              <span key={k} onClick={() => setSnippet(k)} style={{
                padding: "12px 22px", cursor: "pointer",
                fontSize: fs(9), letterSpacing: 2, textTransform: "uppercase",
                color: snippet === k ? "#d8d8d8" : "#8a8a8a",
                background: snippet === k ? "#111" : "transparent",
                borderRight: divider,
                borderBottom: snippet === k ? "1px solid #555" : "1px solid transparent",
              }}>{k}</span>
            ))}
          </div>
          <pre style={{ margin: 0, padding: "26px 28px", fontSize: fs(12), lineHeight: 2, overflowX: "auto" }}>
            {SNIPPETS[snippet].split("\n").map((line: string, i: number) => {
              let c = "#9d9d9d";
              if (line.trim().startsWith("#"))  c = "#777";
              else if (line.includes("curl") || line.includes("ssh")) c = "#d0d0d0";
              else if (line.includes("Authorization") || line.includes("Content-Type")) c = "#a8a8a8";
              else if (line.trim().startsWith('"') || line.trim().startsWith("-d")) c = "#b8b8b8";
              return <div key={i} style={{ color: c }}>{line || " "}</div>;
            })}
          </pre>
        </div>

        <div style={{ marginTop: 48, paddingTop: 18, borderTop: "1px solid #0e0e0e", display: "flex", justifyContent: "space-between", fontSize: fs(10), color: "#737373", letterSpacing: 1 }}>
          <span>hatch v0.1.0-alpha &nbsp;·&nbsp; © 2026</span>
          <span>docs &nbsp;·&nbsp; status &nbsp;·&nbsp; changelog</span>
        </div>
      </div>
    </div>
  );
}