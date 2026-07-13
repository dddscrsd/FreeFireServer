'use strict';

// PostgreSQL implementation of the accounts repository — twin of accounts.mongo.js.
// The auth server only inserts on first Discord login. The game server reads
// `banned`/`authorized` from this table to gate login (src/db/authStore.js), and
// an operator sets bans here (UPDATE auth.accounts SET banned = true WHERE ...).
function createAccountsRepoPg({ pool, schema }) {
  const T = `${schema}.accounts`;

  async function insertAccount(doc) {
    const d = {
      banned: false,
      authorized: true,
      under_analysis: false,
      nickname: null,
      ban_reason: null,
      ...doc,
    };
    await pool.query(
      `INSERT INTO ${T}
         (open_id, uid, nickname, banned, ban_reason, authorized, under_analysis, created_at)
       VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
      [d.open_id, d.uid, d.nickname, d.banned === true, d.ban_reason,
        d.authorized === true, d.under_analysis === true, d.created_at]
    );
    return d;
  }

  async function findByOpenId(openId) {
    const { rows } = await pool.query(
      `SELECT open_id, uid, nickname, banned, ban_reason, authorized, under_analysis, created_at
         FROM ${T} WHERE open_id = $1`,
      [openId]
    );
    const row = rows[0];
    if (!row) return null;
    // uid as string to match the Mongo store; booleans are already JS booleans.
    if (row.uid != null) row.uid = String(row.uid);
    return row;
  }

  return { insertAccount, findByOpenId };
}

module.exports = { createAccountsRepoPg };
