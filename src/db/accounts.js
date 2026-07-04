const fs = require('fs');
const path = require('path');
const crypto = require('crypto');
const Database = require('better-sqlite3');

const config = require('../../config/default');
const logger = require('../logger');

// Account ids start at 10000001 (AUTOINCREMENT seeded to 10000000).
const ACCOUNT_ID_BASE = 10000000;

const DB_FILE =
  (config.db && config.db.file) ||
  path.join(__dirname, '..', '..', 'data', 'accounts.db');

// Ensure the data directory exists.
fs.mkdirSync(path.dirname(DB_FILE), { recursive: true });

const db = new Database(DB_FILE);
db.pragma('journal_mode = WAL');

db.exec(`
  CREATE TABLE IF NOT EXISTS accounts (
    account_id     INTEGER PRIMARY KEY AUTOINCREMENT,
    open_id        TEXT UNIQUE,
    open_id_type   TEXT,
    nickname       TEXT,
    plat_id        INTEGER,
    region         TEXT,
    language       TEXT,
    device_id      TEXT,
    client_version TEXT,
    level          INTEGER DEFAULT 1,
    clan_id        INTEGER DEFAULT 0,
    created_at     INTEGER,
    last_login_at  INTEGER,
    raw_login      TEXT
  );
`);

// --- migration: add columns introduced by the player-state model ----------
// Safe, idempotent. Existing rows/columns are preserved; we only ADD columns
// that are missing. `state` holds the full player document as JSON (see
// src/db/player.js); token/token_created_at index the issued login token so
// player.getByToken() is a single indexed lookup.
const MIGRATION_COLUMNS = [
  ['token', 'TEXT'],
  ['token_created_at', 'INTEGER'],
  ['state', 'TEXT']
];

(function migrateAccounts() {
  const existing = new Set(
    db.prepare('PRAGMA table_info(accounts)').all().map((c) => c.name)
  );
  for (const [name, type] of MIGRATION_COLUMNS) {
    if (!existing.has(name)) {
      db.exec(`ALTER TABLE accounts ADD COLUMN ${name} ${type}`);
      logger.info(`[db] migrated accounts: added column ${name}`);
    }
  }
  // Index the token column for fast Bearer-token resolution.
  db.exec('CREATE INDEX IF NOT EXISTS idx_accounts_token ON accounts(token)');
})();

// Seed the AUTOINCREMENT sequence so the first inserted account_id is 10000001.
// sqlite_sequence exists once an AUTOINCREMENT table has been created above.
try {
  db.prepare(
    `INSERT INTO sqlite_sequence (name, seq)
     SELECT 'accounts', ?
     WHERE NOT EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'accounts')`
  ).run(ACCOUNT_ID_BASE);
} catch (err) {
  logger.warn(`[db] could not seed account_id sequence: ${err.message}`);
}

logger.info(`[db] accounts store ready at ${DB_FILE}`);

const selectByIdStmt = db.prepare('SELECT * FROM accounts WHERE account_id = ?');
const selectByOpenIdStmt = db.prepare('SELECT * FROM accounts WHERE open_id = ?');

const insertStmt = db.prepare(`
  INSERT INTO accounts
    (open_id, open_id_type, nickname, plat_id, region, language, device_id,
     client_version, level, clan_id, created_at, last_login_at, raw_login)
  VALUES
    (@open_id, @open_id_type, @nickname, @plat_id, @region, @language, @device_id,
     @client_version, @level, @clan_id, @created_at, @last_login_at, @raw_login)
`);

// On update we keep existing values when the incoming login omits a field.
const updateStmt = db.prepare(`
  UPDATE accounts SET
    open_id_type   = COALESCE(@open_id_type, open_id_type),
    nickname       = COALESCE(@nickname, nickname),
    plat_id        = COALESCE(@plat_id, plat_id),
    region         = COALESCE(@region, region),
    language       = COALESCE(@language, language),
    device_id      = COALESCE(@device_id, device_id),
    client_version = COALESCE(@client_version, client_version),
    level          = COALESCE(@level, level),
    clan_id        = COALESCE(@clan_id, clan_id),
    last_login_at  = @last_login_at,
    raw_login      = @raw_login
  WHERE open_id = @open_id
`);

function str(v) {
  if (v === undefined || v === null || v === '') return null;
  return String(v);
}

function num(v) {
  if (v === undefined || v === null || v === '') return null;
  const n = Number(v);
  return Number.isFinite(n) ? n : null;
}

// Stable account key: prefer open_id, then device_id, else a generated anon id.
function deriveOpenId(obj) {
  return (
    str(obj.open_id) ||
    str(obj.external_id) ||
    str(obj.device_id) ||
    `anon-${crypto.randomUUID()}`
  );
}

/**
 * Map a LoginReq / PlatformRegisterReq-like object onto the accounts columns.
 */
function toRow(obj, now) {
  return {
    open_id: deriveOpenId(obj),
    open_id_type: str(obj.open_id_type) || str(obj.platform_type),
    nickname: str(obj.nickname),
    plat_id: num(obj.plat_id) ?? num(obj.platform_sdk_id),
    region: str(obj.region),
    language: str(obj.language),
    device_id: str(obj.device_id),
    client_version: str(obj.client_version),
    level: num(obj.level),
    clan_id: num(obj.clan_id),
    created_at: now,
    last_login_at: now,
    raw_login: JSON.stringify(obj || {})
  };
}

/**
 * Get an account row by primary key.
 * @param {number|bigint} accountId
 * @returns {object|undefined}
 */
function getById(accountId) {
  return selectByIdStmt.get(accountId);
}

/**
 * Get an account row by its open_id.
 * @param {string} openId
 * @returns {object|undefined}
 */
function getByOpenId(openId) {
  return selectByOpenIdStmt.get(openId);
}

/**
 * Create-or-update an account from a login/register request object.
 * Returns the resulting account row (including its account_id).
 * @param {object} loginReqObj decoded LoginReq / PlatformRegisterReq plain object
 * @returns {object} account row
 */
function upsertByOpenId(loginReqObj) {
  const now = Date.now();
  const row = toRow(loginReqObj || {}, now);

  const existing = selectByOpenIdStmt.get(row.open_id);
  if (existing) {
    updateStmt.run(row);
    return selectByOpenIdStmt.get(row.open_id);
  }

  const info = insertStmt.run(row);
  return selectByIdStmt.get(info.lastInsertRowid);
}

module.exports = {
  db,
  getById,
  getByOpenId,
  upsertByOpenId,
  ACCOUNT_ID_BASE
};
