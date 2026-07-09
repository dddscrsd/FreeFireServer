'use strict';

// SQLite adapter — an async facade over the synchronous better-sqlite3 store
// (src/db/player.js), so both backends expose ONE interface. This is the default
// until the Postgres cutover, and the baseline the parity harness compares against.
const player = require('../player');
const accounts = require('../accounts');

module.exports = {
  async getByToken(token) {
    return player.getByToken(token);
  },
  async getById(uid) {
    return player.getById(uid);
  },
  async getByOpenId(openId) {
    return player.getByOpenId(openId);
  },
  async getByIds(ids) {
    return (ids || []).map((id) => player.getById(id)).filter(Boolean);
  },
  async searchByNickname(pattern) {
    return accounts.db
      .prepare('SELECT account_id FROM accounts WHERE nickname LIKE ? COLLATE NOCASE LIMIT 20')
      .all(`%${pattern}%`)
      .map((r) => r.account_id);
  },
  async createFromLogin(req) {
    return player.createFromLogin(req);
  },
  async save(account) {
    return player.save(account);
  },
  // Settlement writes the Postgres match_results ledger; the SQLite accounts store has
  // no such table. The settlement worker refuses to start without DATABASE_URL, so this
  // only trips if something misroutes to the SQLite backend.
  async settleMatchResult() {
    throw new Error('settleMatchResult requires the Postgres backend (set DATABASE_URL)');
  },
  async close() {}
};
