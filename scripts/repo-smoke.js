// Smoke test for the Postgres account repository against a REAL Postgres.
// First apply migrations:  node src/db/migrate.js
// Then:                    node scripts/repo-smoke.js       (DATABASE_URL set)
// Creates a throwaway account, exercises every repo method, and cleans up.
require('dotenv').config();

const { Pool } = require('pg');
const config = require('../config/default');
const { PostgresRepo } = require('../src/db/repo/postgres');

const url = process.env.DATABASE_URL || (config.postgres && config.postgres.url);
if (!url) {
  console.error('DATABASE_URL is not set');
  process.exit(1);
}

const OPEN_ID = 'repo-smoke-' + process.pid;

async function main() {
  const repo = new PostgresRepo(url);
  const pool = new Pool({ connectionString: url });
  try {
    const a = await repo.createFromLogin({ open_id: OPEN_ID, nickname: 'SmokeTester', region: 'US', open_id_type: 4 });
    console.log('createFromLogin -> uid=%d nickname=%s coins=%d profiles=%d', a.uid, a.nickname, a.coins, a.profile.profiles.length);

    a.token = 'smoke-tok-' + a.uid;
    a.gems = 4242;
    await repo.save(a);

    const byTok = await repo.getByToken(a.token);
    console.log('getByToken      -> uid=%d gems=%d token=%s', byTok.uid, byTok.gems, byTok.token);
    console.log('getById         -> nickname=%s', (await repo.getById(a.uid)).nickname);
    console.log('getByOpenId     -> uid=%d', (await repo.getByOpenId(OPEN_ID)).uid);
    console.log('searchByNickname-> %j', await repo.searchByNickname('smoketester'));
    console.log('getByIds        -> %d row(s)', (await repo.getByIds([a.uid])).length);
    console.log('OK');
  } finally {
    await pool.query('DELETE FROM accounts WHERE open_id = $1', [OPEN_ID]);
    await pool.end();
    await repo.close();
    console.log('cleaned up test account');
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
