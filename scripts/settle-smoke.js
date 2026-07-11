// Idempotency smoke for the Phase 3 settlement path, against a REAL Postgres (the
// (match_id, account_id) ON CONFLICT idiom can't be modelled by pg-mem, whose
// ON CONFLICT ... RETURNING wrongly returns a row on the no-op). Run on the VPS:
//   node src/db/migrate.js        # ensure the schema exists
//   DATABASE_URL=... node scripts/settle-smoke.js
// Creates two throwaway accounts, delivers a match.result, REDELIVERS the winner,
// and asserts the redelivery does NOT double-credit. Cleans up after itself.
require('dotenv').config();

const { Pool } = require('pg');
const config = require('../config/default');
const { PostgresRepo } = require('../src/db/repo/postgres');

const url = process.env.DATABASE_URL || (config.postgres && config.postgres.url);
if (!url) { console.error('DATABASE_URL is not set'); process.exit(1); }

const TAG = 'settle-smoke-' + process.pid;
const MATCH = TAG;

function assert(cond, msg) { if (!cond) throw new Error('FAIL: ' + msg); }

async function main() {
  const repo = new PostgresRepo(url);
  const pool = new Pool({ connectionString: url });
  const ids = [];
  try {
    const w = await repo.createFromLogin({ open_id: TAG + '-w', nickname: 'SettleW', region: 'US', open_id_type: 4 });
    const l = await repo.createFromLogin({ open_id: TAG + '-l', nickname: 'SettleL', region: 'US', open_id_type: 4 });
    ids.push(w.uid, l.uid);
    const w0 = w.coins || 0;
    const l0 = l.coins || 0;

    const winner = { account_id: w.uid, coins: 250, kills: 3, deaths: 1, win: true,  rank_points: 0, xp: 130 };
    const loser  = { account_id: l.uid, coins: 100, kills: 2, deaths: 3, win: false, rank_points: 0, xp: 60 };

    const r1 = await repo.settleMatchResult(MATCH, winner);
    const r2 = await repo.settleMatchResult(MATCH, loser);
    const r3 = await repo.settleMatchResult(MATCH, winner); // at-least-once redelivery

    const coinsOf = async (id) =>
      Number((await pool.query("SELECT (state->>'coins')::bigint AS c FROM accounts WHERE account_id=$1", [id])).rows[0].c);
    const wc = await coinsOf(w.uid);
    const lc = await coinsOf(l.uid);
    const rows = Number((await pool.query('SELECT count(*)::int AS n FROM match_results WHERE match_id=$1', [MATCH])).rows[0].n);

    console.log('winner  first :', r1, `coins ${w0} -> ${wc}`);
    console.log('loser   first :', r2, `coins ${l0} -> ${lc}`);
    console.log('winner  redup :', r3, '(must not re-credit)');
    console.log('ledger rows   :', rows);

    assert(r1.fresh && r1.credited, 'winner first delivery should credit');
    assert(r2.fresh && r2.credited, 'loser first delivery should credit');
    assert(!r3.fresh && !r3.credited, 'redelivery must be a no-op');
    assert(wc === w0 + 250, `winner coins ${w0}+250, got ${wc}`);
    assert(lc === l0 + 100, `loser coins ${l0}+100, got ${lc}`);
    assert(rows === 2, `ledger should have 2 rows, got ${rows}`);

    console.log('\nSETTLEMENT IDEMPOTENCY OK');
  } finally {
    for (const id of ids) await pool.query('DELETE FROM accounts WHERE account_id=$1', [id]); // FK cascade clears the ledger rows
    await pool.end();
    await repo.close();
    console.log('cleaned up test accounts');
  }
}

main().catch((err) => { console.error(err.message || err); process.exit(1); });
