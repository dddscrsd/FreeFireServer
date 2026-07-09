'use strict';

// Projection reused for token-related reads. We deliberately never fetch
// the whole document unless we have a reason.
const AUTH_PROJECTION = {
  _id: 0,
  open_id: 1,
  uid: 1,
  dc_id: 1,
  dc_handle: 1,
  dc_email: 1,
  access_token: 1,
  refresh_token: 1,
  access_expiry: 1,
  refresh_expiry: 1,
  create_time: 1,
  renovation: 1,
};

function createGuestsRepo(db) {
  const collection = db.collection('guests');

  async function findByDcId(dcId) {
    return collection.findOne({ dc_id: dcId }, { projection: AUTH_PROJECTION });
  }

  async function findByOpenId(openId) {
    return collection.findOne({ open_id: openId }, { projection: AUTH_PROJECTION });
  }

  async function findByAccessToken(token) {
    return collection.findOne({ access_token: token }, { projection: AUTH_PROJECTION });
  }

  async function findByRefreshToken(token) {
    return collection.findOne({ refresh_token: token }, { projection: AUTH_PROJECTION });
  }

  async function findByAccessAndRefreshToken(access, refresh) {
    return collection.findOne(
      { access_token: access, refresh_token: refresh },
      { projection: AUTH_PROJECTION }
    );
  }

  async function insertUser(doc) {
    // Enforce the required renovation default at the repo layer. Any caller
    // that forgets to set it explicitly still ends up with a well-formed doc.
    const withDefaults = { renovation: false, ...doc };
    await collection.insertOne(withDefaults);
    return withDefaults;
  }

  async function updateDiscordFields(openId, { dc_handle, dc_email }) {
    await collection.updateOne(
      { open_id: openId },
      { $set: { dc_handle, dc_email } }
    );
  }

  async function setTokens(openId, { access, refresh, access_expiry, refresh_expiry, create_time, last_login, last_ip }) {
    await collection.updateOne(
      { open_id: openId },
      {
        $set: {
          access_token: access,
          refresh_token: refresh,
          access_expiry,
          refresh_expiry,
          create_time,
          last_login,
          last_ip,
        },
      }
    );
  }

  async function refreshAccessToken(openId, { access, access_expiry }) {
    await collection.updateOne(
      { open_id: openId },
      { $set: { access_token: access, access_expiry } }
    );
  }

  async function clearTokens(openId) {
    await collection.updateOne(
      { open_id: openId },
      { $set: { access_token: null, refresh_token: null } }
    );
  }

  async function currentMaxUid() {
    // uid is stored as a string; convert to numeric at aggregation time so
    // ordering is correct across digit-count boundaries.
    const cursor = collection.aggregate([
      { $match: { uid: { $type: 'string' } } },
      { $group: { _id: null, max: { $max: { $toLong: '$uid' } } } },
    ]);
    const result = await cursor.toArray();
    if (result.length === 0) return null;
    return typeof result[0].max === 'bigint' ? Number(result[0].max) : result[0].max;
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

module.exports = { createGuestsRepo };
