import * as dotenv from "dotenv";

dotenv.config({ path: "../.env" });
/* eslint-disable @typescript-eslint/no-namespace */
declare global {
  namespace NodeJS {
    interface ProcessEnv {

      BETTER_AUTH_URL: string;
      BETTER_AUTH_SECRET: string;
      GOOGLE_CLIENT_ID: string;
      GOOGLE_CLIENT_SECRET: string;

      DATABASE_URL: string;
      HATCH_API_BASE_URL?: string;
    }
  }
}

const config = {
  db: {
    databaseURL: process.env.DATABASE_URL,
  },
  auth: {
    baseURL: process.env.BETTER_AUTH_URL,
    secret: process.env.BETTER_AUTH_SECRET,
    googleClientId: process.env.GOOGLE_CLIENT_ID,
    googleClientSecret: process.env.GOOGLE_CLIENT_SECRET,
  },
  hatch: {
    apiBaseURL: process.env.HATCH_API_BASE_URL || "http://127.0.0.1:8080",
  },
};

export default config;