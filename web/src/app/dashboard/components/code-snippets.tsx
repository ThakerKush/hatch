"use client";

import { useState } from "react";

import { Card } from "@/components/ui/card";

const SNIPPETS: Record<string, { lines: { text: string; dim?: boolean }[] }> = {
  create: {
    lines: [
      { text: "$ curl -X POST https://api.hatchvm.com/v1/vms \\" },
      { text: '    -H "x-hatch-api-key: $HATCH_KEY" \\' },
      { text: '    -H "Content-Type: application/json" \\' },
      { text: `    -d '{"vcpu_count":2,"mem_mib":1024}'` },
      { text: "" },
      { text: '{ "id": "vm-a1b2c3", "state": "running" }', dim: true },
    ],
  },
  list: {
    lines: [
      { text: "$ curl https://api.hatchvm.com/v1/vms \\" },
      { text: '    -H "x-hatch-api-key: $HATCH_KEY"' },
      { text: "" },
      { text: '[ { "id": "vm-a1b2c3", "state": "running" }, ... ]', dim: true },
    ],
  },
  snapshot: {
    lines: [
      { text: "$ curl -X POST https://api.hatchvm.com/v1/vms/vm-a1b2c3/snapshot \\" },
      { text: '    -H "x-hatch-api-key: $HATCH_KEY"' },
      { text: "" },
      { text: '{ "snapshot_id": "snap-x9y8z7", "ok": true }', dim: true },
    ],
  },
  restore: {
    lines: [
      { text: "$ curl -X POST https://api.hatchvm.com/v1/vms/vm-a1b2c3/restore \\" },
      { text: '    -H "x-hatch-api-key: $HATCH_KEY" \\' },
      { text: '    -d \'{"snapshot_id":"snap-x9y8z7"}\'' },
      { text: "" },
      { text: '{ "state": "running", "ok": true }', dim: true },
    ],
  },
};

type SnippetKey = keyof typeof SNIPPETS;

function toPlainText(key: SnippetKey) {
  return SNIPPETS[key].lines.map((l) => l.text).join("\n");
}

export function CodeSnippets() {
  const [active, setActive] = useState<SnippetKey>("create");
  const [copied, setCopied] = useState(false);
  const snippetKeys = Object.keys(SNIPPETS) as SnippetKey[];

  const copy = async () => {
    await navigator.clipboard.writeText(toPlainText(active));
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <Card className="rounded-none border-zinc-800 bg-zinc-950 font-mono">
      {/* title bar */}
      <div className="flex items-center gap-2 border-b border-zinc-800 px-4 py-2.5">
        <span className="h-2.5 w-2.5 rounded-full bg-zinc-700" />
        <span className="h-2.5 w-2.5 rounded-full bg-zinc-700" />
        <span className="h-2.5 w-2.5 rounded-full bg-zinc-700" />
        <span className="ml-2 text-[10px] tracking-widest text-zinc-600">terminal</span>
      </div>

      {/* tab bar */}
      <div className="flex items-center justify-between border-b border-zinc-800">
        <div className="flex">
          {snippetKeys.map((key) => (
            <button
              key={key}
              type="button"
              onClick={() => setActive(key)}
              className={`border-r border-zinc-800 px-5 py-2.5 text-[10px] uppercase tracking-[0.18em] transition-colors ${
                active === key
                  ? "bg-zinc-900 text-zinc-200"
                  : "text-zinc-600 hover:text-zinc-300"
              }`}
            >
              {key}
            </button>
          ))}
        </div>
        <button
          type="button"
          onClick={() => void copy()}
          className="mr-4 text-[10px] uppercase tracking-[0.16em] text-zinc-600 transition-colors hover:text-zinc-300"
        >
          {copied ? "✓ copied" : "copy"}
        </button>
      </div>

      {/* code body */}
      <div className="p-5">
        {SNIPPETS[active].lines.map((line, i) => (
          <div key={i} className="flex min-h-[1.6rem] items-start">
            <span
              className={`block whitespace-pre text-xs leading-6 ${
                line.dim ? "text-zinc-600" : "text-zinc-300"
              }`}
            >
              {line.text || "\u00a0"}
            </span>
          </div>
        ))}
      </div>
    </Card>
  );
}
