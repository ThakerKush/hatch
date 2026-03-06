"use client";

import { useMemo, useState } from "react";

import { Card } from "@/components/ui/card";

const SNIPPETS = {
  create: `curl -X POST http://127.0.0.1:8080/vms \\
  -H "Authorization: Bearer $HATCH_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "vcpu_count": 2,
    "mem_mib": 1024
  }'`,
  list: `curl -X GET http://127.0.0.1:8080/vms \\
  -H "Authorization: Bearer $HATCH_KEY"`,
  snapshot: `curl -X POST http://127.0.0.1:8080/vms/vm_xxx/snapshot \\
  -H "Authorization: Bearer $HATCH_KEY"`,
  restore: `curl -X POST http://127.0.0.1:8080/vms/vm_xxx/restore \\
  -H "Authorization: Bearer $HATCH_KEY"`,
};

type SnippetKey = keyof typeof SNIPPETS;

export function CodeSnippets() {
  const [active, setActive] = useState<SnippetKey>("create");
  const snippetKeys = useMemo(() => Object.keys(SNIPPETS) as SnippetKey[], []);

  return (
    <Card className="rounded-none border-zinc-800 bg-zinc-950">
      <div className="flex border-b border-zinc-800 font-mono">
        {snippetKeys.map((key) => (
          <button
            key={key}
            type="button"
            onClick={() => setActive(key)}
            className={`border-r border-zinc-800 px-5 py-3 text-[10px] uppercase tracking-[0.18em] ${
              active === key ? "text-zinc-200" : "text-zinc-500 hover:text-zinc-300"
            }`}
          >
            {key}
          </button>
        ))}
      </div>
      <pre className="overflow-x-auto p-6 font-mono text-xs leading-7 text-zinc-400">{SNIPPETS[active]}</pre>
    </Card>
  );
}
