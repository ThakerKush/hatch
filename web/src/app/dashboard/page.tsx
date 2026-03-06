import { headers } from "next/headers";
import { redirect } from "next/navigation";

import { DashboardShell } from "@/app/dashboard/components/dashboard-shell";
import { auth } from "@/lib/auth";

export default async function DashboardPage() {
  const session = await auth.api.getSession({
    headers: await headers(),
  });
  if (!session) {
    redirect("/");
  }

  return <DashboardShell userLabel={session.user.email || "user"} />;
}