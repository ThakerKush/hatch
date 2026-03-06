import { betterAuth } from "better-auth";
import { nextCookies } from "better-auth/next-js";
import config from "@/config";
import { pool } from "@/lib/db";
import { apiKey } from "@better-auth/api-key";

export const auth = betterAuth({
  baseURL: config.auth.baseURL,
  secret: config.auth.secret,
  database: pool,
  trustedOrigins: [
    config.auth.baseURL || "http://localhost:3000",
    "https://hatchvm.com",
    "https://console.hatchvm.com",
  ],
  rateLimit: {
    window: 60,
    max: 1000,
  },
  socialProviders: {
    google: {
      clientId: config.auth.googleClientId as string,
      clientSecret: config.auth.googleClientSecret as string,
      prompt: "select_account",
    },
  },
  plugins: [
    apiKey({
      defaultPrefix: "hatch_",
      enableMetadata: false,
      requireName: false,
      rateLimit: {
        enabled: false,
      },
    }),
    nextCookies(),
  ],
});
