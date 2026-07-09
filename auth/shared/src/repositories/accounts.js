'use strict';

// The `accounts` collection carries game data (nickname, banned, ban_reason,
// authorized, under_analysis, plus whatever the game server writes there).
// The auth server only inserts documents on first Discord login; it never
// modifies existing accounts documents.
function createAccountsRepo(db) {
  const collection = db.collection('accounts');

  async function insertAccount(doc) {
    const withDefaults = {
      banned: false,
      authorized: true,
      under_analysis: false,
      nickname: null,
      ban_reason: null,
      ...doc,
    };
    await collection.insertOne(withDefaults);
    return withDefaults;
  }

  async function findByOpenId(openId) {
    return collection.findOne({ open_id: openId }, { projection: { _id: 0 } });
  }

  return { insertAccount, findByOpenId };
}

module.exports = { createAccountsRepo };
