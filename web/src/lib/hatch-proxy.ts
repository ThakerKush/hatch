import { NextRequest, NextResponse } from "next/server";

import config from "@/config";
import { auth } from "@/lib/auth";

function extractProvidedAPIKey(req: NextRequest): string {
  const fromHeader = req.headers.get("x-hatch-api-key");
  if (fromHeader && fromHeader.trim()) {
    return fromHeader.trim();
  }

  const authHeader = req.headers.get("authorization");
  if (authHeader && authHeader.startsWith("Bearer ")) {
    return authHeader.slice("Bearer ".length).trim();
  }

  return "";
}

async function validateOwnedAPIKey(req: NextRequest): Promise<string | null> {
  const session = await auth.api.getSession({ headers: req.headers });
  if (!session?.user?.id) {
    return null;
  }

  const key = extractProvidedAPIKey(req);
  if (!key) {
    return null;
  }

  const { valid, key: apiKey } = await auth.api.verifyApiKey({ body: { key } });
  if (!valid || apiKey?.referenceId !== session.user.id) {
    return null;
  }

  return key;
}

export async function proxyToHatch(
  req: NextRequest,
  upstreamPath: string,
): Promise<NextResponse> {
  const apiKey = await validateOwnedAPIKey(req);
  if (!apiKey) {
    return NextResponse.json(
      {
        error:
          "Missing or invalid Hatch API key. Create a key and connect it to the dashboard.",
      },
      { status: 401 },
    );
  }

  const upstreamURL = new URL(upstreamPath, config.hatch.apiBaseURL);
  const upstreamResponse = await fetch(upstreamURL, {
    method: "GET",
    headers: {
      Authorization: `Bearer ${apiKey}`,
      Accept: "application/json",
    },
    cache: "no-store",
  });

  const payload = await upstreamResponse.text();
  return new NextResponse(payload, {
    status: upstreamResponse.status,
    headers: {
      "content-type": upstreamResponse.headers.get("content-type") || "application/json",
    },
  });
}
