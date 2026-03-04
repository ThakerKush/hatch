import { Pool } from "pg";
import config from "@/config";
const globalForPool = globalThis as unknown as { pool?: Pool };

export const pool =
  globalForPool.pool ??
  new Pool({
    connectionString: config.db.databaseURL,
  });

if (process.env.NODE_ENV !== "production") {
  globalForPool.pool = pool;
}
