/**
 * GetFriend  (-> AccountFriendRes)
 *
 * Ported from ported_8.js (handleGetFriend). reference: get_friend @
 * htpp.py:7073 — load the caller's friend id list, fetch each friend's account,
 * map to AccountInfoWithPresence (accountToPresence, now in _shared).
 *
 * (registered with no explicit types in the source, so reqType/resType are null
 * and resolved from endpoint_map.json by the router.)
 */

'use strict';

const { requireAccount, accountToPresence } = require('./_shared');
const player = require('../db/player');

function handler(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  const friendIds = Array.isArray(account.friends) ? account.friends : [];
  const now = Math.floor(Date.now() / 1000);

  const friends = [];
  for (const fid of friendIds) {
    const id = typeof fid === 'object' ? fid.account_id || fid.uid : fid;
    const acc = id ? player.getById(id) : null;
    if (acc) friends.push(accountToPresence(acc, now));
  }
  ctx.logger.info(`[ported] GetFriend uid=${account.uid} -> ${friends.length} friends`);
  return { friends, star_friends: [], friends_alias_info: [] };
}

module.exports = {
  endpoint: 'GetFriend',
  reqType: null,
  resType: null,
  handler
};
