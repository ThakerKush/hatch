import { headers } from "next/headers";
import { redirect } from "next/navigation";

import { auth } from "@/lib/auth";

import { LandingPage } from "@/app/landing";

export default async function Home() {
  const session = await auth.api.getSession({
    headers: await headers(),
  });
  if (session) {
    // Redirect to the console subdomain — redirecting to "/dashboard" here would
    // be caught by the middleware on hatchvm.com and sent back to "/", looping.
    const consoleUrl =
      process.env.NODE_ENV === "production"
        ? "https://console.hatchvm.com/dashboard"
        : "/dashboard";
    redirect(consoleUrl);
  }

  return <LandingPage />;
}
