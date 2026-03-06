import { NextRequest } from "next/server";

import { proxyToHatch } from "@/lib/hatch-proxy";

export async function GET(req: NextRequest) {
  return proxyToHatch(req, "/vms");
}
