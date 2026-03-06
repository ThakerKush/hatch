"use client";

import { useState } from "react";

import { authClient } from "@/lib/auth-client";

import type { NavTab } from "./types";

type DashboardNavbarProps = {
  activeTab: NavTab;
  onTabChange: (tab: NavTab) => void;
  userLabel: string;
};

const tabs: NavTab[] = ["dashboard"];

export function DashboardNavbar({ activeTab, onTabChange, userLabel }: DashboardNavbarProps) {
  const [signingOut, setSigningOut] = useState(false);

  const signOut = async () => {
    setSigningOut(true);
    await authClient.signOut({ fetchOptions: { onSuccess: () => { window.location.href = "/"; } } });
    setSigningOut(false);
  };

  return (
    <header className="sticky top-0 z-20 border-b border-zinc-800 bg-black/90 backdrop-blur">
      <div className="mx-auto flex max-w-6xl items-center justify-between px-8 py-4 font-mono">
        <div className="flex items-center gap-3">
          <span className="text-zinc-200">▣</span>
          <span className="text-sm font-bold tracking-[0.3em] text-zinc-100">HATCH</span>
          <span className="text-zinc-700">│</span>
          <span className="text-[10px] tracking-[0.2em] text-zinc-500">MICRO VM ORCHESTRATION</span>
        </div>

        <nav className="flex gap-7 text-[11px] uppercase tracking-[0.16em]">
          {tabs.map((tab) => (
            <button
              key={tab}
              type="button"
              onClick={() => onTabChange(tab)}
              className={`border-b pb-1 transition-colors ${
                activeTab === tab
                  ? "border-zinc-500 text-zinc-100"
                  : "border-transparent text-zinc-500 hover:text-zinc-300"
              }`}
            >
              {tab}
            </button>
          ))}
        </nav>

        <div className="flex items-center gap-4 text-[11px] tracking-[0.12em] text-zinc-500">
          <span>
            <span className="text-zinc-400">●</span> {userLabel} · api v1
          </span>
          <button
            type="button"
            disabled={signingOut}
            onClick={() => void signOut()}
            className="border border-zinc-800 px-3 py-1 text-[10px] uppercase tracking-[0.16em] text-zinc-600 transition-colors hover:border-zinc-600 hover:text-zinc-300 disabled:opacity-40"
          >
            {signingOut ? "..." : "sign out"}
          </button>
        </div>
      </div>
    </header>
  );
}
