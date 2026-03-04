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
            Sign in with Google to create and manage API keys.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <GoogleSignInButton />
          <p className="text-xs text-zinc-500">
            API docs and VM operations are available at{" "}
            <Link className="underline" href="https://api.hatchvm.com/healthz">
              api.hatchvm.com
            </Link>
            .
          </p>
        </CardContent>
      </Card>
    </div>
  );
}
