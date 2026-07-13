'use strict';

// One-shot data migration: copy the auth store's guests + accounts from MongoDB
// into PostgreSQL (the pairings collection is ephemeral — TTL'd in minutes — so
// it is intentionally skipped). Re-runnable: rows upsert by open_id, so running
// it twice is a no-op the second time.
//
// Needs BOTH backends configured at once:
//   MONGO_URI + DB_NAME   -> source (the old Mongo store)
//   DATABASE_URL          -> destination (Postgres; AUTH_PG_SCHEMA picks the schema)
//
// Usage (from the auth/ workspace):
//   MONGO_URI=mongodb://... DB_NAME=libmadoka_auth DATABASE_URL=postgres://... \
//     node scripts/migrate-mongo-to-pg.js [--dry-run]
//   # or: npm run migrate:pg
//
// It ensures the Postgres schema/tables exist first (via @auth/shared), so you do
// not need to have started the auth server on Postgres beforehand.

require('dotenv').config();

const { config, mongo, store, logger } = require('@auth/shared');

// --- coercion helpers (Mongo doc -> Postgres column) ------------------------
const bool = (v, d = false) => (v === true ? true : v === false ? false : d);
const orNull = (v) => (v === undefined ? null : v);
// uid was stored as a string in Mongo; Postgres BIGINT accepts a numeric string.
const uidOf = (doc) => (doc.uid === undefined || doc.uid === null ? null : String(doc.uid));

const GUEST_COLS = [
  'open_id', 'uid', 'dc_id', 'dc_handle', 'dc_email', 'renovation',
  'access_token', 'refresh_token', 'access_expiry', 'refresh_expiry', 'create_time',
  'last_login', 'last_ip', 'registered_at', 'registered_ip', 'renovated_at', 'renovated_ip',
];
const ACCOUNT_COLS = [
  'open_id', 'uid', 'nickname', 'banned', 'ban_reason', 'authorized', 'under_analysis', 'created_at',
];

// Build an idempotent "INSERT ... ON CONFLICT (open_id) DO UPDATE" for a column list.
function upsertSql(table, cols) {
  const ph = cols.map((_, i) => `$${i + 1}`).join(', ');
  const upd = cols.filter((c) => c !== 'open_id').map((c) => `${c} = EXCLUDED.${c}`).join(', ');
  return `INSERT INTO ${table} (${cols.join(', ')}) VALUES (${ph})
          ON CONFLICT (open_id) DO UPDATE SET ${upd}`;
}

function guestValues(d) {
  return [
    d.open_id, uidOf(d), orNull(d.dc_id), orNull(d.dc_handle), orNull(d.dc_email), bool(d.renovation),
    orNull(d.access_token), orNull(d.refresh_token), orNull(d.access_expiry), orNull(d.refresh_expiry),
    orNull(d.create_time), orNull(d.last_login), orNull(d.last_ip), orNull(d.registered_at),
    orNull(d.registered_ip), orNull(d.renovated_at), orNull(d.renovated_ip),
  ];
}

function accountValues(d) {
  return [
    d.open_id, uidOf(d), orNull(d.nickname), bool(d.banned), orNull(d.ban_reason),
    bool(d.authorized, true), bool(d.under_analysis), orNull(d.created_at),
  ];
}

// Migrate one collection. Injectable (db, pool) so it can be unit-tested with fakes.
// Rows are upserted one at a time and errors are tolerated (counted) so a single
// malformed doc — e.g. a duplicate uid — can't abort the whole run.
async function migrateCollection({ db, pool, schema, name, cols, toValues, valid, log, dryRun }) {
  const docs = await db.collection(name).find({}, { projection: { _id: 0 } }).toArray();
  log(`[auth-migrate] ${name}: read ${docs.length} document(s)`);
  const sql = upsertSql(`${schema}.${name}`, cols);
  let ok = 0, skipped = 0, failed = 0;
  for (const d of docs) {
    if (!valid(d)) { skipped++; continue; }
    if (dryRun) { ok++; continue; }
    try {
      await pool.query(sql, toValues(d));
      ok++;
    } catch (e) {
      failed++;
      log(`[auth-migrate] ${name}: row open_id=${d.open_id} failed: ${e.message}`);
    }
  }
  log(`[auth-migrate] ${name}: upserted ${ok}, skipped ${skipped}, failed ${failed}${dryRun ? ' (dry-run)' : ''}`);
  return { ok, skipped, failed };
}

// Preflight: report uids shared by more than one open_id. In Mongo a string
// "10000001" and a number 10000001 are distinct (so its unique index allowed
// both), but the Postgres BIGINT column collapses them — surfacing here as either
// a relaxed accounts row (fine) or, for guests (uid stays UNIQUE), a hard failure
// worth cleaning up. Best-effort: never blocks the migration.
async function reportDupUids(db, name, log) {
  try {
    const dups = await db.collection(name).aggregate([
      { $group: {
        _id: { $convert: { input: '$uid', to: 'long', onError: null, onNull: null } },
        open_ids: { $addToSet: '$open_id' }, n: { $sum: 1 } } },
      { $match: { _id: { $ne: null }, n: { $gt: 1 } } },
      { $sort: { n: -1 } },
    ]).toArray();
    if (!dups.length) return dups;
    log(`[auth-migrate] NOTE: ${name} has ${dups.length} uid(s) shared by >1 open_id (Mongo string/number uids collapse in BIGINT):`);
    for (const d of dups.slice(0, 20)) log(`  uid=${d._id} <- open_ids ${JSON.stringify(d.open_ids)}`);
    if (dups.length > 20) log(`  ...and ${dups.length - 20} more`);
    return dups;
  } catch (e) {
    log(`[auth-migrate] dup-uid preflight for ${name} skipped: ${e.message}`);
    return [];
  }
}

async function migrateAll({ db, pool, schema, log = console.log, dryRun = false }) {
  await reportDupUids(db, 'guests', log);
  await reportDupUids(db, 'accounts', log);
  const guests = await migrateCollection({
    db, pool, schema, name: 'guests', cols: GUEST_COLS, toValues: guestValues,
    valid: (d) => d.open_id && uidOf(d) != null, log, dryRun,
  });
  const accounts = await migrateCollection({
    db, pool, schema, name: 'accounts', cols: ACCOUNT_COLS, toValues: accountValues,
    valid: (d) => !!d.open_id, log, dryRun,
  });
  log('[auth-migrate] pairings: skipped (ephemeral / TTL — recreated on demand)');
  return { guests, accounts };
}

async function main() {
  const dryRun = process.argv.includes('--dry-run');

  if (config.DB_BACKEND !== 'pg' || !config.DATABASE_URL) {
    logger.error('[auth-migrate] DATABASE_URL is not set — nothing to migrate INTO');
    process.exit(1);
  }
  if (!config.MONGO_URI || !config.DB_NAME) {
    logger.error('[auth-migrate] MONGO_URI + DB_NAME must be set (the source Mongo store)');
    process.exit(1);
  }

  const { db } = await mongo.connect();
  const handle = await store.connect();      // { backend:'pg', pool, schema }
  await store.ensureSchema(handle);          // create schema/tables if missing
  logger.info(`[auth-migrate] source=mongo(${config.DB_NAME}) dest=pg(schema=${handle.schema})${dryRun ? ' [dry-run]' : ''}`);

  try {
    const res = await migrateAll({ db, pool: handle.pool, schema: handle.schema, log: (m) => logger.info(m), dryRun });
    const bad = res.guests.failed + res.accounts.failed;
    logger.info(`[auth-migrate] done — guests(${res.guests.ok}) accounts(${res.accounts.ok}) failed(${bad})`);
    process.exitCode = bad > 0 ? 2 : 0;
  } catch (err) {
    logger.error(`[auth-migrate] failed: ${err.stack || err.message}`);
    process.exitCode = 1;
  } finally {
    try { await mongo.close(); } catch (e) { /* ignore */ }
    try { await store.close(); } catch (e) { /* ignore */ }
  }
}

if (require.main === module) main();

module.exports = { migrateAll, migrateCollection, upsertSql, guestValues, accountValues };
