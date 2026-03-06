"use client";

import { useState } from "react";

import { Button } from "@/components/ui/button";
import { authClient } from "@/lib/auth-client";

export function GoogleSignInButton() {
  const [loading, setLoading] = useState(false);

  return (
    <Button
      onClick={async () => {
        setLoading(true);
        await authClient.signIn.social({
          provider: "google",
          callbackURL: "/",
        });
        setLoading(false);
      }}
      disabled={loading}
      className="w-full"
    >
      {loading ? "Signing in..." : "Continue with Google"}
    </Button>
  );
}
