import { auth } from "@/lib/auth";
import { NextRequest, NextResponse } from "next/server";

export async function POST(req: NextRequest) {
  const body = await req.json();
  const key = body?.key;

  if (!key || typeof key !== "string") {
    return NextResponse.json({ valid: false, error: "missing key" }, { status: 400 });
  }

  try {
    const result = await auth.api.verifyApiKey({ body: { key } });
    return NextResponse.json({ valid: result.valid });
  } catch {
    return NextResponse.json({ valid: false }, { status: 200 });
  }
}
