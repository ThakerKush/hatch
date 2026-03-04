"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Check, Copy, Trash2, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { authClient } from "@/lib/auth-client";

type Props = {
  initialKeys: APIKeyRecord[];
};

type APIKeyRecord = {
  id: string;
  name: string | null;
  start: string | null;
  enabled: boolean;
};

function KeyRevealModal({
  token,
  onClose,
}: {
  token: string;
  onClose: () => void;
}) {
  const [copied, setCopied] = useState(false);

  async function copyKey() {
    await navigator.clipboard.writeText(token);
    setCopied(true);
  }

  useEffect(() => {
    if (!copied) return;
    const t = setTimeout(() => setCopied(false), 2000);
    return () => clearTimeout(t);
  }, [copied]);

  const handleBackdrop = useCallback(
    (e: React.MouseEvent) => {
      if (e.target === e.currentTarget) onClose();
    },
    [onClose],
  );

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm"
      onClick={handleBackdrop}
    >
      <div className="w-full max-w-lg rounded-lg border border-zinc-800 bg-zinc-950 p-6 shadow-2xl">
        <div className="mb-4 flex items-center justify-between">
          <h3 className="text-sm font-medium text-zinc-100">
            Your new API key
          </h3>
          <button
            onClick={onClose}
            className="rounded p-1 text-zinc-500 hover:bg-zinc-800 hover:text-zinc-300"
          >
            <X className="size-4" />
          </button>
        </div>

        <p className="mb-4 text-xs text-zinc-500">
          Copy this key now. You won&#39;t be able to see it again.
        </p>

        <div className="rounded-md border border-zinc-800 bg-black p-3">
          <code className="block break-all text-xs text-zinc-200">
            {token}
          </code>
        </div>

        <div className="mt-4 flex justify-end">
          <Button
            variant={copied ? "outline" : "default"}
            onClick={copyKey}
            className={copied ? "border-green-700 text-green-400" : ""}
          >
            {copied ? (
              <>
                <Check className="size-4" />
                Copied
              </>
            ) : (
              <>
                <Copy className="size-4" />
                Copy to clipboard
              </>
            )}
          </Button>
        </div>
      </div>
    </div>
  );
}

export function KeyManager({ initialKeys }: Props) {
  const [keys, setKeys] = useState(initialKeys);
  const [name, setName] = useState("");
  const [newToken, setNewToken] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const activeCount = useMemo(
    () => keys.filter((key) => key.enabled).length,
    [keys],
  );

  async function refreshKeys() {
    const { data } = await authClient.apiKey.list({});
    setKeys(
      (data?.apiKeys ?? []).map((key) => ({
        id: key.id,
        name: key.name,
        start: key.start,
        enabled: key.enabled,
      })),
    );
  }

  async function createKey() {
    setLoading(true);
    const { data } = await authClient.apiKey.create({
      name: name.trim() || "default",
    });
    if (data?.key) {
      setNewToken(data.key);
      setName("");
      await refreshKeys();
    }
    setLoading(false);
  }

  async function revokeKey(id: string) {
    await authClient.apiKey.delete({
      keyId: id,
    });
    await refreshKeys();
  }

  return (
    <div className="space-y-8">
      {newToken && (
        <KeyRevealModal
          token={newToken}
          onClose={() => setNewToken(null)}
        />
      )}

      <div className="space-y-3">
        <p className="text-xs uppercase tracking-wide text-zinc-500">
          Active keys: {activeCount}
        </p>
        <div className="flex gap-2">
          <Input
            placeholder="Key name (optional)"
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
          <Button onClick={createKey} disabled={loading}>
            Create key
          </Button>
        </div>
      </div>

      <div className="space-y-3">
        {keys.length === 0 ? (
          <p className="text-sm text-zinc-500">No API keys yet.</p>
        ) : (
          keys.map((key) => (
            <div
              key={key.id}
              className="flex items-center justify-between rounded-md border border-zinc-900 px-3 py-2"
            >
              <div className="space-y-1">
                <p className="text-sm text-zinc-200">{key.name}</p>
                <p className="text-xs text-zinc-500">
                  {key.start ?? "hidden"}... • {key.enabled ? "active" : "disabled"}
                </p>
              </div>
              {key.enabled ? (
                <Button
                  size="sm"
                  variant="destructive"
                  onClick={() => revokeKey(key.id)}
                >
                  <Trash2 className="size-4" />
                  Revoke
                </Button>
              ) : null}
            </div>
          ))
        )}
      </div>
    </div>
  );
}
