import { headers } from "next/headers";
import { redirect } from "next/navigation";

import { SignOutButton } from "@/components/auth/sign-out-button";
import { KeyManager } from "@/components/keys/key-manager";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { auth } from "@/lib/auth";

export default async function DashboardPage() {
  const session = await auth.api.getSession({
    headers: await headers(),
  });
  if (!session) {
    redirect("/");
  }

  const keyResult = await auth.api.listApiKeys({
    headers: await headers(),
  });
  const keys = keyResult.apiKeys.map((key) => ({
    id: key.id,
    name: key.name,
    start: key.start,
    enabled: key.enabled,
  }));

  return (
    <main className="mx-auto min-h-screen w-full max-w-3xl p-6">
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-zinc-100">API Keys</h1>
          <p className="text-sm text-zinc-500">{session.user.email}</p>
        </div>
        <SignOutButton />
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Access keys</CardTitle>
          <CardDescription>
            Create a key, then send it as <code>Authorization: Bearer &lt;key&gt;</code>.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <KeyManager initialKeys={keys} />
        </CardContent>
      </Card>
    </main>
  );
}
