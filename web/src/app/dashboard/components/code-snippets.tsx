"use client";

import { useState } from "react";

import { Card } from "@/components/ui/card";

type SnippetLine = { text: string; dim?: boolean };

type Snippet = {
  lines: SnippetLine[];
  copyText: string;
};

// user_data value encoded as a JSON string literal:
// \n → JSON newline, \" → JSON escaped double-quote.
// Wrapped in single quotes for the curl -d argument so the shell passes it verbatim.
const USER_DATA =
  "#cloud-config\\nhostname: dev-vm\\nusers:\\n" +
  "  - name: hatch\\n    groups: [sudo]\\n    shell: /bin/bash\\n" +
  '    sudo: [\\"ALL=(ALL) NOPASSWD:ALL\\"]\\n' +
  "    ssh_authorized_keys:\\n      - YOUR_SSH_PUBLIC_KEY\\n" +
  "packages:\\n  - python3\\n  - golang-go\\n  - curl";

const SNIPPETS: Record<string, Snippet> = {
  create: {
    lines: [
      { text: "$ curl -X POST https://api.hatchvm.com/vms \\" },
      { text: '    -H "Authorization: Bearer $HATCH_API_KEY" \\' },
      { text: '    -H "Content-Type: application/json" \\' },
      { text: `    -d '{"vcpu_count":2,"mem_mib":1024,` },
      { text: `       "user_data":"#cloud-config\\nhostname: dev-vm\\n..."}'` },
      { text: "" },
      { text: '{ "id": "vm-a1b2c3", "state": "running" }', dim: true },
    ],
    copyText:
      "curl -X POST https://api.hatchvm.com/vms \\\n" +
      '    -H "Authorization: Bearer $HATCH_API_KEY" \\\n' +
      '    -H "Content-Type: application/json" \\\n' +
      `    -d '{"vcpu_count":2,"mem_mib":1024,"user_data":"${USER_DATA}"}'`,
  },
  list: {
    lines: [
      { text: "$ curl https://api.hatchvm.com/vms \\" },
      { text: '    -H "Authorization: Bearer $HATCH_API_KEY"' },
      { text: "" },
      { text: '[{ "id": "vm-a1b2c3", "state": "running" }, ...]', dim: true },
    ],
    copyText:
      "curl https://api.hatchvm.com/vms \\\n" +
      '    -H "Authorization: Bearer $HATCH_API_KEY"',
  },
  snapshot: {
    lines: [
      { text: "$ curl -X POST https://api.hatchvm.com/vms/vm-a1b2c3/snapshot \\" },
      { text: '    -H "Authorization: Bearer $HATCH_API_KEY"' },
      { text: "" },
      { text: '{ "id": "snap-x9y8z7", "vm_id": "vm-a1b2c3" }', dim: true },
    ],
    copyText:
      "curl -X POST https://api.hatchvm.com/vms/vm-a1b2c3/snapshot \\\n" +
      '    -H "Authorization: Bearer $HATCH_API_KEY"',
  },
  restore: {
    lines: [
      { text: "$ curl -X POST https://api.hatchvm.com/vms/vm-a1b2c3/restore \\" },
      { text: '    -H "Authorization: Bearer $HATCH_API_KEY"' },
      { text: "" },
      { text: '{ "id": "vm-a1b2c3", "state": "running" }', dim: true },
    ],
    copyText:
      "curl -X POST https://api.hatchvm.com/vms/vm-a1b2c3/restore \\\n" +
      '    -H "Authorization: Bearer $HATCH_API_KEY"',
  },
};

type SnippetKey = keyof typeof SNIPPETS;

export function CodeSnippets() {
  const [active, setActive] = useState<SnippetKey>("create");
  const [copied, setCopied] = useState(false);
  const snippetKeys = Object.keys(SNIPPETS) as SnippetKey[];

  const copy = async () => {
    await navigator.clipboard.writeText(SNIPPETS[active].copyText);
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
