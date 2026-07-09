'use strict';

// PostgreSQL account repository — the async mirror of src/db/player.js. The rich
// player document lives in accounts.state (JSONB); the indexed columns are kept
// in sync on write. Behaviour matches player.js exactly (same getters, same
// createFromLogin insert-then-fill, same denormalised columns) so the two stores
// can run side by side for parity before the cutover.
//
// NOTE (Phase 1): DEFAULT_PLAYER / deriveOpenId are borrowed from player.js, which
// pulls in the SQLite module. Step 3 extracts them to a store-agnostic accountDoc
// module so the Postgres path no longer depends on better-sqlite3.
const { Pool } = require('pg');

const { DEFAULT_PLAYER, deriveOpenId } = require('../player');
const logger = require('../../logger');

// Turn an accounts row into the rich player document. `state` (JSONB) is already
// a parsed object from node-postgres; overlay the authoritative index columns.
function rowToAccount(row) {
  if (!row) return null;
  const hasState = row.state && typeof row.state === 'object' && Object.keys(row.state).length > 0;
  const acc = hasState ? row.state : DEFAULT_PLAYER();
  acc.uid = Number(row.account_id);
  acc.account_id = Number(row.account_id);
  if (row.open_id != null) acc.open_id = row.open_id;
  if (row.token != null) acc.token = row.token;
  if (row.token_created_at != null) acc.token_created_at = Number(row.token_created_at);
  return acc;
}

// The denormalised columns extracted from the account doc, kept in sync on save.
function denorm(account) {
  return {
    open_id_type: account.open_id_type ?? null,
    nickname: account.nickname ?? null,
    region: account.region ?? null,
    language: account.language ?? null,
    device_id: account.device_id ?? null,
    client_version: account.client_version ?? null,
    level: account.level ?? 1,
    clan_id: (account.clan && account.clan.id) || account.clan_id || 0,
    token: account.token ?? null,
    token_created_at: account.token_created_at ?? 0
  };
}

class PostgresRepo {
  // pool is injectable for testing (pg-mem); otherwise built from the URL.
  constructor(url, pool) {
    this.pool = pool || new Pool({ connectionString: url });
    if (this.pool.on) this.pool.on('error', (err) => logger.error(`[repo:pg] pool: ${err.message}`));
  }

  async getByToken(token) {
    if (!token) return null;
    const { rows } = await this.pool.query('SELECT * FROM accounts WHERE token = $1', [token]);
    return rowToAccount(rows[0]);
  }

  async getById(uid) {
    if (!uid) return null;
    const { rows } = await this.pool.query('SELECT * FROM accounts WHERE account_id = $1', [uid]);
    return rowToAccount(rows[0]);
  }

  async getByOpenId(openId) {
    if (!openId) return null;
    const { rows } = await this.pool.query('SELECT * FROM accounts WHERE open_id = $1', [openId]);
    return rowToAccount(rows[0]);
  }

  // Batch fetch — replaces the N+1 per-id loops in the friend/clan fan-outs. Uses
  // a bound IN list (portable + plenty efficient for the bounded friend/clan sizes).
  async getByIds(ids) {
    if (!ids || !ids.length) return [];
    const placeholders = ids.map((_, i) => `$${i + 1}`).join(',');
    const { rows } = await this.pool.query(
      `SELECT * FROM accounts WHERE account_id IN (${placeholders})`,
      ids
    );
    return rows.map(rowToAccount);
  }

  async searchByNickname(pattern) {
    const { rows } = await this.pool.query(
      'SELECT account_id FROM accounts WHERE nickname ILIKE $1 LIMIT 20',
      [`%${pattern}%`]
    );
    return rows.map((r) => Number(r.account_id));
  }

  async save(account) {
    if (!account) return account;
    const accountId = account.uid || account.account_id;
    if (!accountId) throw new Error('repo.save: account has no uid/account_id');
    account.uid = accountId;
    account.account_id = accountId;
    account.last_login_at = account.last_login_at || Date.now();
    const d = denorm(account);
    await this.pool.query(
      `UPDATE accounts SET
         open_id_type=$2, nickname=$3, region=$4, language=$5, device_id=$6,
         client_version=$7, level=$8, clan_id=$9, last_login_at=$10,
         token=$11, token_created_at=$12, state=$13::jsonb
       WHERE account_id=$1`,
      [
        accountId, d.open_id_type, d.nickname, d.region, d.language, d.device_id,
        d.client_version, d.level, d.clan_id, account.last_login_at, d.token,
        d.token_created_at, JSON.stringify(account)
      ]
    );
    return account;
  }

  async createFromLogin(loginReqObj) {
    const req = loginReqObj || {};
    const openId = deriveOpenId(req);
    const now = Date.now();

    const existing = await this.getByOpenId(openId);
    if (existing) {
      if (req.open_id_type != null) existing.open_id_type = String(req.open_id_type);
      if (req.nickname) existing.nickname = String(req.nickname);
      if (req.region) existing.region = String(req.region);
      if (req.language) existing.language = String(req.language);
      if (req.device_id) existing.device_id = String(req.device_id);
      if (req.client_version) existing.client_version = String(req.client_version);
      return this.save(existing);
    }

    // Insert a placeholder to obtain the identity account_id, then persist the doc.
    const { rows } = await this.pool.query(
      `INSERT INTO accounts
         (open_id, open_id_type, nickname, region, language, device_id,
          client_version, level, clan_id, created_at, last_login_at,
          token, token_created_at, raw_login, state)
       VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::jsonb,$15::jsonb)
       RETURNING account_id`,
      [
        openId,
        req.open_id_type != null ? String(req.open_id_type) : null,
        req.nickname ? String(req.nickname) : 'Player',
        req.region ? String(req.region) : null,
        req.language ? String(req.language) : null,
        req.device_id ? String(req.device_id) : null,
        req.client_version ? String(req.client_version) : null,
        100, 0, now, now, null, 0, JSON.stringify(req), '{}'
      ]
    );

    const acc = DEFAULT_PLAYER();
    acc.uid = Number(rows[0].account_id);
    acc.account_id = acc.uid;
    acc.open_id = openId;
    acc.open_id_type = req.open_id_type != null ? String(req.open_id_type) : null;
    if (req.nickname) acc.nickname = String(req.nickname);
    acc.region = req.region ? String(req.region) : null;
    acc.language = req.language ? String(req.language) : null;
    acc.device_id = req.device_id ? String(req.device_id) : null;
    acc.client_version = req.client_version ? String(req.client_version) : null;
    acc.created_at = now;
    acc.last_login_at = now;

    logger.info(`[repo:pg] created account uid=${acc.uid} open_id=${openId}`);
    return this.save(acc);
  }

  async close() {
    if (this.pool.end) await this.pool.end();
  }
}

module.exports = { PostgresRepo, rowToAccount, denorm };
