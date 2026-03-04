"use client";

import { useState } from "react";

import { Button } from "@/components/ui/button";
import { authClient } from "@/lib/auth-client";

export function SignOutButton() {
  const [loading, setLoading] = useState(false);

  return (
    <Button
      variant="outline"
      onClick={async () => {
        setLoading(true);
        await authClient.signOut();
        window.location.href = "/";
      }}
      disabled={loading}
    >
      {loading ? "Signing out..." : "Sign out"}
    </Button>
  );
}
