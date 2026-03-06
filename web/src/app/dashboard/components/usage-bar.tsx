"use client";

type UsageBarProps = {
  value: number;
};

export function UsageBar({ value }: UsageBarProps) {
  const bounded = Math.max(0, Math.min(100, Math.round(value)));
  const filled = Math.round(bounded / 5);
  const dimmed = bounded === 0;
  const barColor = dimmed
    ? "text-zinc-700"
    : bounded >= 90
      ? "text-zinc-300"
      : bounded >= 70
        ? "text-zinc-400"
        : "text-zinc-500";

  return (
    <span className="font-mono tracking-[0.05em]">
      <span className={barColor}>{"█".repeat(filled)}</span>
      <span className="text-zinc-800">{"░".repeat(20 - filled)}</span>
      <span className={`ml-2 ${dimmed ? "text-zinc-600" : "text-zinc-400"}`}>{bounded}%</span>
    </span>
  );
}
