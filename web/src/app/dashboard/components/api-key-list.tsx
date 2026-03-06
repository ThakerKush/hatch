"use client";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";

import type { DashboardApiKey } from "./types";

type ApiKeyListProps = {
  keys: DashboardApiKey[];
  loading: boolean;
  onCreateClick: () => void;
  onRevoke: (keyId: string) => void;
};

export function ApiKeyList({ keys, loading, onCreateClick, onRevoke }: ApiKeyListProps) {
  return (
    <Card className="rounded-none border-zinc-800 bg-zinc-950">
      <div className="flex items-center justify-between border-b border-zinc-800 px-5 py-3 font-mono">
        <span className="text-[10px] tracking-[0.24em] text-zinc-500">API KEYS</span>
        <Button variant="ghost" size="sm" className="h-7 px-2 font-mono text-xs" onClick={onCreateClick}>
          + new key
        </Button>
      </div>

      {loading ? (
        <div className="px-5 py-6 font-mono text-sm text-zinc-500">Loading keys...</div>
      ) : keys.length === 0 ? (
        <div className="px-5 py-6 font-mono text-sm text-zinc-500">
          No keys yet. Create one to unlock real dashboard data.
        </div>
      ) : (
        keys.map((key) => (
          <div key={key.id} className="border-b border-zinc-900 px-5 py-3 font-mono last:border-b-0">
            <div className="mb-1 flex items-center justify-between">
              <span className="text-sm text-zinc-200">{key.name || "unnamed-key"}</span>
              <span className="text-[11px] text-zinc-600">used {key.lastUsed}</span>
            </div>
            <div className="flex items-center justify-between gap-3">
              <span className="text-xs text-zinc-400">{key.prefix}</span>
              <button
                type="button"
                className="text-xs text-zinc-500 hover:text-zinc-300"
                onClick={() => onRevoke(key.id)}
              >
                revoke
              </button>
            </div>
          </div>
        ))
      )}
    </Card>
  );
}
