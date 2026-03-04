import * as dotenv from "dotenv";

dotenv.config({ path: "../.env" });
declare global {
  namespace NodeJS {
    interface ProcessEnv {

      BETTER_AUTH_URL: string;
      BETTER_AUTH_SECRET: string;
      GOOGLE_CLIENT_ID: string;
      GOOGLE_CLIENT_SECRET: string;

      DATABASE_URL: string;
    }
  }
}

export default {
  db: {
    databaseURL: process.env.DATABASE_URL,
  },
  auth: {
    baseURL: process.env.BETTER_AUTH_URL,
    secret: process.env.BETTER_AUTH_SECRET,
    googleClientId: process.env.GOOGLE_CLIENT_ID,
    googleClientSecret: process.env.GOOGLE_CLIENT_SECRET,
  },
};