import { headers } from "next/headers";

import { GoogleSignInButton } from "@/components/auth/google-sign-in-button";
import { SignOutButton } from "@/components/auth/sign-out-button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { auth } from "@/lib/auth";

export default async function Home() {
  const session = await auth.api.getSession({
    headers: await headers(),
  });

  return (
    <div className="flex min-h-screen items-center justify-center p-6">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>Hatch</CardTitle>
          <CardDescription>
            {session
              ? `Signed in as ${session.user.email}`
              : "Sign in with Google to get started."}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {session ? <SignOutButton /> : <GoogleSignInButton />}
        </CardContent>
      </Card>
    </div>
  );
}
