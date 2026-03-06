import { Card, CardContent, CardHeader } from "@/components/ui/card";

type StatCardProps = {
  label: string;
  value: string;
  subText: string;
};

export function StatCard({ label, value, subText }: StatCardProps) {
  return (
    <Card className="rounded-none border-zinc-800 bg-zinc-950">
      <CardHeader className="space-y-2 p-5 pb-0">
        <p className="text-[10px] uppercase tracking-[0.28em] text-zinc-500">{label}</p>
      </CardHeader>
      <CardContent className="p-5 pt-3">
        <p className="font-mono text-3xl font-light text-zinc-100">{value}</p>
        <p className="mt-1 text-xs text-zinc-500">{subText}</p>
      </CardContent>
    </Card>
  );
}
