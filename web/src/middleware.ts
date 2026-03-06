import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

const CONSOLE_HOST = "console.hatchvm.com";
const ROOT_HOSTS = new Set(["hatchvm.com", "www.hatchvm.com"]);

export function middleware(request: NextRequest) {
  const host = request.headers.get("host") ?? "";
  const { pathname } = request.nextUrl;

  // console.hatchvm.com — rewrite every request so / becomes /dashboard,
  // /api/* and Next.js internals pass through untouched.
  if (host === CONSOLE_HOST) {
    if (pathname === "/") {
      return NextResponse.rewrite(new URL("/dashboard", request.url));
    }
    return NextResponse.next();
  }

  // hatchvm.com / www.hatchvm.com — landing only; redirect /dashboard to /
  if (ROOT_HOSTS.has(host)) {
    if (pathname.startsWith("/dashboard")) {
      return NextResponse.redirect(new URL("/", request.url));
    }
    return NextResponse.next();
  }

  // localhost or any other host (dev) — no host-based routing, pass through
  return NextResponse.next();
}

export const config = {
  matcher: [
    // Skip Next.js internals and static files
    "/((?!_next/static|_next/image|favicon.ico).*)",
  ],
};
