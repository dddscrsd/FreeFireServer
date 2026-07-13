'use strict';

// PostgreSQL implementation of the guests repository — the twin of
// guests.mongo.js. Selected by the dispatcher in guests.js when DATABASE_URL is
// set. Returns documents shaped EXACTLY like the Mongo repo (same field names,
// same JS types) so the route/flow code can't tell the backends apart:
//   - uid comes back as a string (node-postgres renders BIGINT as text, matching
//     the Mongo store which kept uid as a string),
//   - the token-expiry columns are coerced back to JS numbers (they're stored as
//     BIGINT unix-seconds; the callers do numeric `now > access_expiry` compares),
//   - renovation is a real boolean.

// The columns exposed to callers (== the Mongo AUTH_PROJECTION).
const SELECT_COLS =
  'open_id, uid, dc_id, dc_handle, dc_email, access_token, refresh_token, ' +
  'access_expiry, refresh_expiry, create_time, renovation';

function mapGuest(row) {
  if (!row) return null;
  return {
    open_id: row.open_id,
    uid: row.uid == null ? null : String(row.uid),
    dc_id: row.dc_id,
    dc_handle: row.dc_handle,
    dc_email: row.dc_email,
    access_token: row.access_token,
    refresh_token: row.refresh_token,
    access_expiry: row.access_expiry == null ? null : Number(row.access_expiry),
    refresh_expiry: row.refresh_expiry == null ? null : Number(row.refresh_expiry),
    create_time: row.create_time == null ? null : Number(row.create_time),
    renovation: row.renovation === true,
  };
}

function createGuestsRepoPg({ pool, schema }) {
  const T = `${schema}.guests`;

  async function one(sql, params) {
    const { rows } = await pool.query(sql, params);
    return mapGuest(rows[0]);
  }

  async function findByDcId(dcId) {
    return one(`SELECT ${SELECT_COLS} FROM ${T} WHERE dc_id = $1`, [dcId]);
  }

  async function findByOpenId(openId) {
    return one(`SELECT ${SELECT_COLS} FROM ${T} WHERE open_id = $1`, [openId]);
  }

  async function findByAccessToken(token) {
    if (!token) return null;
    return one(`SELECT ${SELECT_COLS} FROM ${T} WHERE access_token = $1`, [token]);
  }

  async function findByRefreshToken(token) {
    if (!token) return null;
    return one(`SELECT ${SELECT_COLS} FROM ${T} WHERE refresh_token = $1`, [token]);
  }

  async function findByAccessAndRefreshToken(access, refresh) {
    if (!access || !refresh) return null;
    return one(
      `SELECT ${SELECT_COLS} FROM ${T} WHERE access_token = $1 AND refresh_token = $2`,
      [access, refresh]
    );
  }

  // Insert a fresh guest. The caller (login-flow) builds the full document; we
  // persist the known columns. A concurrent race on uid/dc_id raises a unique
  // violation (SQLSTATE 23505) which ids.withUidRetry catches and retries.
  async function insertUser(doc) {
    const d = { renovation: false, ...doc };
    await pool.query(
      `INSERT INTO ${T}
         (open_id, uid, dc_id, dc_handle, dc_email, renovation,
          access_token, refresh_token, access_expiry, refresh_expiry, create_time,
          last_login, last_ip, registered_at, registered_ip, renovated_at, renovated_ip)
       VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
      [
        d.open_id, d.uid, d.dc_id, d.dc_handle, d.dc_email, d.renovation === true,
        d.access_token, d.refresh_token, d.access_expiry, d.refresh_expiry, d.create_time,
        d.last_login, d.last_ip, d.registered_at, d.registered_ip, d.renovated_at, d.renovated_ip,
      ]
    );
    return d;
  }

  async function updateDiscordFields(openId, { dc_handle, dc_email }) {
    await pool.query(
      `UPDATE ${T} SET dc_handle = $2, dc_email = $3 WHERE open_id = $1`,
      [openId, dc_handle, dc_email]
    );
  }

  async function setTokens(openId, { access, refresh, access_expiry, refresh_expiry, create_time, last_login, last_ip }) {
    await pool.query(
      `UPDATE ${T} SET
         access_token = $2, refresh_token = $3, access_expiry = $4, refresh_expiry = $5,
         create_time = $6, last_login = $7, last_ip = $8
       WHERE open_id = $1`,
      [openId, access, refresh, access_expiry, refresh_expiry, create_time, last_login, last_ip]
    );
  }

  async function refreshAccessToken(openId, { access, access_expiry }) {
    await pool.query(
      `UPDATE ${T} SET access_token = $2, access_expiry = $3 WHERE open_id = $1`,
      [openId, access, access_expiry]
    );
  }

  async function clearTokens(openId) {
    await pool.query(
      `UPDATE ${T} SET access_token = NULL, refresh_token = NULL WHERE open_id = $1`,
      [openId]
    );
  }

  async function currentMaxUid() {
    const { rows } = await pool.query(`SELECT MAX(uid) AS max FROM ${T}`);
    const max = rows[0] && rows[0].max;
    return max == null ? null : Number(max);
  }

  return {
    findByDcId,
    findByOpenId,
    findByAccessToken,
    findByRefreshToken,
    findByAccessAndRefreshToken,
    insertUser,
    updateDiscordFields,
    setTokens,
    refreshAccessToken,
    clearTokens,
    currentMaxUid,
  };
}

module.exports = { createGuestsRepoPg, mapGuest };
