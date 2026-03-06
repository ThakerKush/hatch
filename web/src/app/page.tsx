import { headers } from "next/headers";
import Link from "next/link";
import { redirect } from "next/navigation";

import { GoogleSignInButton } from "@/components/auth/google-sign-in-button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { auth } from "@/lib/auth";

export default async function Home() {
  const session = await auth.api.getSession({
    headers: await headers(),
  });
  if (session) {
    redirect("/dashboard");
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-6">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>Hatch</CardTitle>
          <CardDescription>
            Sign in with Google to get started.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            <GoogleSignInButton />
            <Link href="/dashboard" className="block text-center text-xs text-zinc-400 hover:text-zinc-200">
              view dashboard
            </Link>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
