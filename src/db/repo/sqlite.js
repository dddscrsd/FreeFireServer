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

  // --- friends / friend-requests (blob-backed; mirrors the pre-relational behaviour) --
  async getFriendIds(uid) {
    const a = player.getById(uid);
    return a && Array.isArray(a.friends) ? a.friends.map(Number) : [];
  },
  async getRequestIds(uid) {
    const a = player.getById(uid);
    return a && Array.isArray(a.requests) ? a.requests.map(Number) : [];
  },
  async addRequest(targetId, fromId) {
    if (!targetId || !fromId || Number(targetId) === Number(fromId)) return;
    const t = player.getById(targetId);
    if (!t) return;
    t.requests = Array.isArray(t.requests) ? t.requests : [];
    if (!t.requests.some((r) => Number(r) === Number(fromId))) { t.requests.push(Number(fromId)); player.save(t); }
  },
  async removeRequest(targetId, fromId) {
    const t = player.getById(targetId);
    if (!t || !Array.isArray(t.requests)) return;
    const before = t.requests.length;
    t.requests = t.requests.filter((r) => Number(r) !== Number(fromId));
    if (t.requests.length !== before) player.save(t);
  },
  async addFriendship(a, b) {
    if (!a || !b || Number(a) === Number(b)) return;
    for (const [self, other] of [[a, b], [b, a]]) {
      const acc = player.getById(self);
      if (!acc) continue;
      acc.friends = Array.isArray(acc.friends) ? acc.friends : [];
      acc.requests = Array.isArray(acc.requests) ? acc.requests.filter((r) => Number(r) !== Number(other)) : [];
      if (!acc.friends.some((f) => Number(f) === Number(other))) acc.friends.push(Number(other));
      player.save(acc);
    }
  },
  async removeFriendship(a, b) {
    if (!a || !b) return;
    for (const [self, other] of [[a, b], [b, a]]) {
      const acc = player.getById(self);
      if (!acc || !Array.isArray(acc.friends)) continue;
      acc.friends = acc.friends.filter((f) => Number(f) !== Number(other));
      player.save(acc);
    }
  },

  async close() {}
};
