'use strict';

// Read-only client to the AUTH SERVER's store (auth.guests / auth.accounts),
// used by the game login/register handlers to enforce "only accounts registered
// on the auth server may join" and to read ban flags. Shares the same Postgres as
// the game (DATABASE_URL); the auth tables live in their own schema (config.auth
// .pgSchema, default `auth`) so there's no clash with the game's public.accounts.
//
// It is DISABLED (all lookups return null, isEnabled() === false) when there is no
// DATABASE_URL — so the SQLite dev flow is unaffected. Lookups throw on a real DB
// error; callers in _authGate.js catch and FAIL OPEN (a transient auth-DB blip
// must not lock every player out), logging loudly.

const config = require('../../config/default');
const logger = require('../logger');

let initialized = false;
let enabled = false;
let pool = null;
let schema = 'auth';

function ensure() {
  if (initialized) return;
  initialized = true;
  const url = config.postgres && config.postgres.url;
  const s = (config.auth && config.auth.pgSchema) || 'auth';
  schema = /^[A-Za-z_][A-Za-z0-9_]*$/.test(s) ? s : 'auth';
  if (!url) {
    logger.info('[authStore] disabled (no DATABASE_URL) — auth-store gates are inert');
    return;
  }
  try {
    const { Pool } = require('pg');
    pool = new Pool({ connectionString: url, max: 5, idleTimeoutMillis: 30_000 });
    pool.on('error', (e) => logger.error(`[authStore] pool: ${e.message}`));
    enabled = true;
    logger.info(`[authStore] enabled (schema=${schema})`);
  } catch (e) {
    logger.error(`[authStore] init failed: ${e.message}`);
  }
}

function isEnabled() {
  ensure();
  return enabled;
}

// The guests row (identity) for an open_id, or null. `access_token` is the game
// client's login_token. Throws on DB error.
async function guestByOpenId(openId) {
  ensure();
  if (!enabled || !openId) return null;
  const { rows } = await pool.query(
    `SELECT open_id, uid, access_token, renovation, access_expiry
       FROM ${schema}.guests WHERE open_id = $1`,
    [openId]
  );
  return rows[0] || null;
}

// The accounts row (game data + moderation flags) for an open_id, or null.
// Throws on DB error.
async function accountByOpenId(openId) {
  ensure();
  if (!enabled || !openId) return null;
  const { rows } = await pool.query(
    `SELECT open_id, uid, banned, ban_reason, authorized
       FROM ${schema}.accounts WHERE open_id = $1`,
    [openId]
  );
  return rows[0] || null;
}

async function close() {
  if (pool && pool.end) {
    try { await pool.end(); } finally { pool = null; enabled = false; }
  }
}

module.exports = { isEnabled, guestByOpenId, accountByOpenId, close };
