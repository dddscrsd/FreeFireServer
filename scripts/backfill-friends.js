// One-shot backfill: populate the relational friendships + friend_requests tables from
// each Postgres account's state blob (state.friends / state.requests), so the friend
// handlers (now reading the tables) see the existing data. Idempotent — safe to re-run.
// Inserts BOTH friendship directions so a historically one-sided blob becomes consistent.
//   node src/db/migrate.js            # ensure the tables exist
//   DATABASE_URL=... node scripts/backfill-friends.js
require('dotenv').config();

const { Pool } = require('pg');
const config = require('../config/default');

const url = process.env.DATABASE_URL || (config.postgres && config.postgres.url);
if (!url) { console.error('DATABASE_URL is not set'); process.exit(1); }

async function insertSkippingFk(pool, sql, params) {
  try {
    const r = await pool.query(sql, params);
    return r.rowCount || 0;
  } catch (e) {
    if (e.code === '23503') return 0; // FK violation: friend/requester account no longer exists — skip
    throw e;
  }
}

async function main() {
  const pool = new Pool({ connectionString: url });
  try {
    const { rows } = await pool.query('SELECT account_id, state FROM accounts');
    const now = Date.now();
    let friendships = 0;
    let requests = 0;
    for (const row of rows) {
      const uid = Number(row.account_id);
      const st = row.state && typeof row.state === 'object' ? row.state : {};
      const friends = Array.isArray(st.friends) ? st.friends.map(Number).filter(Boolean) : [];
      const reqs = Array.isArray(st.requests) ? st.requests.map(Number).filter(Boolean) : [];
      for (const f of friends) {
        if (f === uid) continue;
        friendships += await insertSkippingFk(pool,
          'INSERT INTO friendships (a,b,created_at) VALUES ($1,$2,$3),($2,$1,$3) ON CONFLICT DO NOTHING',
          [uid, f, now]);
      }
      for (const from of reqs) {
        if (from === uid) continue;
        requests += await insertSkippingFk(pool,
          'INSERT INTO friend_requests (target_id, from_id, created_at) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING',
          [uid, from, now]);
      }
    }
    console.log(`backfilled ${friendships} friendship row(s) + ${requests} request row(s) from ${rows.length} account(s)`);
  } finally {
    await pool.end();
  }
}

main().catch((err) => { console.error(err.message || err); process.exit(1); });
