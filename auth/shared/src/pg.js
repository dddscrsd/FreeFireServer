'use strict';

// PostgreSQL backend for the auth server. The Mongo path (mongo.js) is kept as a
// fallback; this file is only used when DATABASE_URL is set (config.DB_BACKEND ===
// 'pg'). It mirrors mongo.js's connect / ensureSchema / close surface so the
// bootstraps stay backend-agnostic (see store.js).
//
// All three tables live in a dedicated schema (config.PG_SCHEMA, default `auth`)
// so they never collide with the game server's public.accounts, which has the
// same name but a different shape. The repositories (repositories/*.pg.js) read
// the schema from the handle returned by connect().

const { Pool } = require('pg');
const config = require('./config');
const { logger } = require('./logger');

let pool = null;
let sweeper = null;

// The schema name is an operator-controlled config value, but it is interpolated
// into DDL/DML unquoted, so validate it defensively (belt-and-braces: reject
// anything that isn't a plain SQL identifier).
function safeSchema() {
  const s = config.PG_SCHEMA;
  if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(s)) {
    throw new Error(`invalid AUTH_PG_SCHEMA '${s}' (must be a plain SQL identifier)`);
  }
  return s;
}

async function connect() {
  if (pool) return { pool, schema: safeSchema() };
  const schema = safeSchema();
  pool = new Pool({
    connectionString: config.DATABASE_URL,
    max: 20,
    idleTimeoutMillis: 30_000,
    connectionTimeoutMillis: 5_000,
  });
  pool.on('error', (err) => logger.error({ err }, 'pg pool error'));
  // Fail fast if the DB is unreachable, matching mongo.connect()'s behaviour.
  const client = await pool.connect();
  client.release();
  return { pool, schema };
}

// Create the schema, tables and indexes if they do not already exist. Idempotent,
// so every service can call it on boot (mirrors mongo.ensureIndexes).
async function ensureSchema() {
  const { pool: p, schema } = await connect();
  await p.query(`CREATE SCHEMA IF NOT EXISTS ${schema}`);

  // guests: the identity records. `access_token` doubles as the game client's
  // login_token. uid is BIGINT (node-postgres returns it as a string, matching
  // the Mongo store which kept uid as a string). The numeric token-expiry columns
  // are BIGINT unix-seconds; the repo coerces them back to JS numbers on read.
  await p.query(`
    CREATE TABLE IF NOT EXISTS ${schema}.guests (
      open_id        TEXT PRIMARY KEY,
      uid            BIGINT UNIQUE NOT NULL,
      dc_id          TEXT UNIQUE,
      dc_handle      TEXT,
      dc_email       TEXT,
      renovation     BOOLEAN NOT NULL DEFAULT FALSE,
      access_token   TEXT,
      refresh_token  TEXT,
      access_expiry  BIGINT,
      refresh_expiry BIGINT,
      create_time    BIGINT,
      last_login     TEXT,
      last_ip        TEXT,
      registered_at  TEXT,
      registered_ip  TEXT,
      renovated_at   TEXT,
      renovated_ip   TEXT
    )`);
  await p.query(`CREATE INDEX IF NOT EXISTS guests_access_token_idx ON ${schema}.guests (access_token)`);
  await p.query(`CREATE INDEX IF NOT EXISTS guests_refresh_token_idx ON ${schema}.guests (refresh_token)`);

  // accounts: game-facing data + moderation flags. The game server reads
  // `banned`/`authorized` here to gate login (see src/db/authStore.js).
  await p.query(`
    CREATE TABLE IF NOT EXISTS ${schema}.accounts (
      open_id        TEXT PRIMARY KEY,
      uid            BIGINT UNIQUE,
      nickname       TEXT,
      banned         BOOLEAN NOT NULL DEFAULT FALSE,
      ban_reason     TEXT,
      authorized     BOOLEAN NOT NULL DEFAULT TRUE,
      under_analysis BOOLEAN NOT NULL DEFAULT FALSE,
      created_at     TEXT
    )`);

  // pairings: short-lived OAuth bridges. Mongo auto-expired these with a TTL
  // index; Postgres has none, so reads filter on expires_at and a sweeper deletes
  // stale rows (startPairingSweeper).
  await p.query(`
    CREATE TABLE IF NOT EXISTS ${schema}.pairings (
      pair_id      TEXT PRIMARY KEY,
      redirect_uri TEXT,
      status       TEXT NOT NULL,
      redirect_url TEXT,
      error        TEXT,
      created_at   TIMESTAMPTZ NOT NULL,
      expires_at   TIMESTAMPTZ NOT NULL
    )`);
  await p.query(`CREATE INDEX IF NOT EXISTS pairings_expires_at_idx ON ${schema}.pairings (expires_at)`);

  startPairingSweeper();
  logger.info({ schema }, 'pg schema ensured');
}

// Periodically delete expired pairings (Postgres has no TTL index). Runs on the
// first service to call ensureSchema; unref'd so it never holds the process open.
function startPairingSweeper() {
  if (sweeper) return;
  const schema = safeSchema();
  const tick = () => {
    if (!pool) return;
    pool
      .query(`DELETE FROM ${schema}.pairings WHERE expires_at <= now()`)
      .catch((err) => logger.warn({ err }, 'pairings sweep failed'));
  };
  sweeper = setInterval(tick, config.PAIRING_SWEEP_INTERVAL_MS);
  if (sweeper.unref) sweeper.unref();
}

async function close() {
  if (sweeper) {
    clearInterval(sweeper);
    sweeper = null;
  }
  if (pool) {
    try {
      await pool.end();
    } finally {
      pool = null;
    }
  }
}

module.exports = { connect, ensureSchema, close };
