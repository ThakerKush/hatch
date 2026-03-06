import { NextRequest } from "next/server";

import { proxyToHatch } from "@/lib/hatch-proxy";

type RouteContext = {
  params: Promise<{
    id: string;
  }>;
};

export async function GET(req: NextRequest, context: RouteContext) {
  const { id } = await context.params;
  return proxyToHatch(req, `/vms/${id}/snapshots`);
}
