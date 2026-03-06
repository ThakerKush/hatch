import { headers } from "next/headers";
import { redirect } from "next/navigation";

import { DashboardShell } from "@/app/dashboard/components/dashboard-shell";
import { auth } from "@/lib/auth";

export default async function DashboardPage() {
  const session = await auth.api.getSession({
    headers: await headers(),
  });
  if (!session) {
    // Redirect to the landing domain, not "/" — on console.hatchvm.com "/" gets
    // rewritten back to /dashboard by middleware, causing an infinite loop.
    const landingUrl =
      process.env.BETTER_AUTH_URL?.replace("console.", "") ?? "/";
    redirect(landingUrl);
  }

  return <DashboardShell userLabel={session.user.email || "user"} />;
}