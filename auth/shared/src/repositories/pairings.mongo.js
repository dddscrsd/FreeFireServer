'use strict';

// MongoDB implementation of the pairings repository. The `pairings` collection
// bridges the game's WebView and the browser that completes the Discord OAuth
// flow. Rows expire automatically via a TTL index on `expires_at`.
//
// See pairings.js for the dispatcher and pairings.pg.js for the Postgres twin
// (which sweeps expired rows on a timer, since Postgres has no TTL index).

const PENDING = 'pending';
const COMPLETED = 'completed';
const FAILED = 'failed';

function createPairingsRepoMongo(db) {
  const collection = db.collection('pairings');

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
    await collection.insertOne(doc);
    return doc;
  }

  async function findByPairId(pairId) {
    return collection.findOne({ pair_id: pairId }, { projection: { _id: 0 } });
  }

  // Marks a pending pairing completed and attaches the fbconnect URL. Uses
  // findOneAndUpdate with a pending-status filter so a completed row is
  // never overwritten (e.g., if the callback fires twice).
  async function markCompleted(pairId, { redirectUrl }) {
    const result = await collection.findOneAndUpdate(
      { pair_id: pairId, status: PENDING },
      { $set: { status: COMPLETED, redirect_url: redirectUrl } },
      { returnDocument: 'after', projection: { _id: 0 } }
    );
    return result || null;
  }

  async function markFailed(pairId, { error }) {
    const result = await collection.findOneAndUpdate(
      { pair_id: pairId, status: PENDING },
      { $set: { status: FAILED, error: String(error).slice(0, 200) } },
      { returnDocument: 'after', projection: { _id: 0 } }
    );
    return result || null;
  }

  // Called by /pair/status. If the row is still pending, returns
  // { status: 'pending' } without side effects. If the row is completed or
  // failed, deletes it atomically and returns the terminal status +
  // payload. If no row exists (expired or unknown), returns
  // { status: 'expired' }.
  async function consumeIfCompleted(pairId) {
    const doc = await collection.findOne({ pair_id: pairId });
    if (!doc) return { status: 'expired' };
    if (doc.status === PENDING) return { status: PENDING };

    // Atomic single-use consumption of the terminal row. The filter
    // guarantees we do not delete a row that has since been re-created for
    // a different flow (pair_id collisions are astronomically unlikely
    // given 32 bytes of randomness, but the filter makes it explicit).
    await collection.deleteOne({ pair_id: pairId, status: doc.status });

    if (doc.status === COMPLETED) {
      return { status: COMPLETED, redirect_url: doc.redirect_url };
    }
    if (doc.status === FAILED) {
      return { status: FAILED, error: doc.error };
    }
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

module.exports = { createPairingsRepoMongo };
