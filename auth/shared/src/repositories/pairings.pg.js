'use strict';

// PostgreSQL implementation of the pairings repository — twin of pairings.mongo.js.
// Postgres has no TTL index, so reads treat rows past `expires_at` as absent and
// pg.js runs a sweeper that deletes them (startPairingSweeper). Semantics match
// the Mongo repo exactly.

const PENDING = 'pending';
const COMPLETED = 'completed';
const FAILED = 'failed';

function mapPairing(row) {
  if (!row) return null;
  return {
    pair_id: row.pair_id,
    redirect_uri: row.redirect_uri,
    status: row.status,
    redirect_url: row.redirect_url,
    error: row.error,
    created_at: row.created_at,
    expires_at: row.expires_at,
  };
}

function createPairingsRepoPg({ pool, schema }) {
  const T = `${schema}.pairings`;

  async function createPending({ pairId, redirectUri, ttlSeconds = 600, now = new Date() }) {
    const expiresAt = new Date(now.getTime() + ttlSeconds * 1000);
    const doc = {
      pair_id: pairId,
      redirect_uri: redirectUri,
      status: PENDING,
      redirect_url: null,
      error: null,
      created_at: now,
      expires_at: expiresAt,
    };
    await pool.query(
      `INSERT INTO ${T} (pair_id, redirect_uri, status, redirect_url, error, created_at, expires_at)
       VALUES ($1,$2,$3,$4,$5,$6,$7)`,
      [doc.pair_id, doc.redirect_uri, doc.status, doc.redirect_url, doc.error, doc.created_at, doc.expires_at]
    );
    return doc;
  }

  // Expired rows are treated as absent (they may not have been swept yet).
  async function findByPairId(pairId) {
    const { rows } = await pool.query(
      `SELECT * FROM ${T} WHERE pair_id = $1 AND expires_at > now()`,
      [pairId]
    );
    return mapPairing(rows[0]);
  }

  async function markCompleted(pairId, { redirectUrl }) {
    const { rows } = await pool.query(
      `UPDATE ${T} SET status = $2, redirect_url = $3
         WHERE pair_id = $1 AND status = $4 AND expires_at > now()
       RETURNING *`,
      [pairId, COMPLETED, redirectUrl, PENDING]
    );
    return mapPairing(rows[0]);
  }

  async function markFailed(pairId, { error }) {
    const { rows } = await pool.query(
      `UPDATE ${T} SET status = $2, error = $3
         WHERE pair_id = $1 AND status = $4 AND expires_at > now()
       RETURNING *`,
      [pairId, FAILED, String(error).slice(0, 200), PENDING]
    );
    return mapPairing(rows[0]);
  }

  // Pending -> report pending (no side effects). Terminal -> delete atomically
  // and return the payload. Missing/expired -> 'expired'.
  async function consumeIfCompleted(pairId) {
    const sel = await pool.query(
      `SELECT status FROM ${T} WHERE pair_id = $1 AND expires_at > now()`,
      [pairId]
    );
    const row = sel.rows[0];
    if (!row) return { status: 'expired' };
    if (row.status === PENDING) return { status: PENDING };

    // Atomic single-use consumption: DELETE ... RETURNING guarantees exactly one
    // caller observes the terminal payload.
    const del = await pool.query(
      `DELETE FROM ${T} WHERE pair_id = $1 AND status = $2 RETURNING status, redirect_url, error`,
      [pairId, row.status]
    );
    const consumed = del.rows[0];
    if (!consumed) return { status: 'expired' };
    if (consumed.status === COMPLETED) return { status: COMPLETED, redirect_url: consumed.redirect_url };
    if (consumed.status === FAILED) return { status: FAILED, error: consumed.error };
    return { status: 'expired' };
  }

  return {
    createPending,
    findByPairId,
    markCompleted,
    markFailed,
    consumeIfCompleted,
    _statuses: { PENDING, COMPLETED, FAILED },
  };
}

module.exports = { createPairingsRepoPg };
