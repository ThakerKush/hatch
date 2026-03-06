"use client";

type UsageBarProps = {
  value: number;
};

export function UsageBar({ value }: UsageBarProps) {
  const bounded = Math.max(0, Math.min(100, Math.round(value)));
  const dimmed = bounded === 0;

  const barFill = dimmed
    ? "bg-zinc-800"
    : bounded >= 90
      ? "bg-zinc-300"
      : bounded >= 70
        ? "bg-zinc-400"
        : "bg-zinc-500";

  return (
    <span className="flex items-center gap-2 font-mono">
      <span className="relative h-2 w-full max-w-[120px] overflow-hidden rounded-[1px] bg-zinc-800/60">
        <span
          className={`absolute inset-y-0 left-0 ${barFill} transition-[width] duration-300`}
          style={{ width: `${bounded}%` }}
        />
      </span>
      <span className={`min-w-[3ch] text-right text-[11px] tabular-nums ${dimmed ? "text-zinc-600" : "text-zinc-400"}`}>
        {bounded}%
      </span>
    </span>
  );
}
