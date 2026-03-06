"use client";

import { useEffect, useState } from "react";
import { Check, Copy } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";

type CreateKeyResult = {
  plainTextKey: string;
};

type CreateKeyModalProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreateKey: (name: string) => Promise<CreateKeyResult>;
};

export function CreateKeyModal({ open, onOpenChange, onCreateKey }: CreateKeyModalProps) {
  const [name, setName] = useState("");
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState("");
  const [createdKey, setCreatedKey] = useState("");
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!open) {
      setName("");
      setCreating(false);
      setError("");
      setCreatedKey("");
      setCopied(false);
    }
  }, [open]);

  const submit = async () => {
    if (!name.trim()) {
      setError("Name is required.");
      return;
    }

    setCreating(true);
    setError("");
    try {
      const result = await onCreateKey(name.trim());
      setCreatedKey(result.plainTextKey);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to create API key");
    } finally {
      setCreating(false);
    }
  };

  const copyKey = async () => {
    if (!createdKey) {
      return;
    }
    await navigator.clipboard.writeText(createdKey);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="font-mono">
        <DialogHeader>
          <DialogTitle className="tracking-[0.16em] uppercase">Create API key</DialogTitle>
          <DialogDescription className="text-zinc-500">
            Generate a key for dashboard and API usage.
          </DialogDescription>
        </DialogHeader>

        {!createdKey ? (
          <div className="space-y-4">
            <Input
              autoFocus
              value={name}
              onChange={(event) => setName(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter") {
                  void submit();
                }
              }}
              placeholder="key name (e.g. prod-worker)"
              className="font-mono"
            />
            {error ? <p className="text-sm text-red-300">{error}</p> : null}
            <DialogFooter>
              <Button variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button onClick={() => void submit()} disabled={creating}>
                {creating ? "Creating..." : "Create key"}
              </Button>
            </DialogFooter>
          </div>
        ) : (
          <div className="min-w-0 space-y-4">
            <p className="text-sm text-zinc-400">Copy this key now. It will never be shown again.</p>
            <div className="rounded-md border border-zinc-800 bg-zinc-950 p-3">
              <code className="block break-all text-xs leading-5 text-zinc-200">{createdKey}</code>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => void copyKey()}>
                {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
                {copied ? "Copied" : "Copy"}
              </Button>
              <Button onClick={() => onOpenChange(false)}>Done</Button>
            </DialogFooter>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
