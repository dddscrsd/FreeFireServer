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
const { getRepo } = require('../db/repo');

async function handler(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  const friendIds = Array.isArray(account.friends) ? account.friends : [];
  const now = Math.floor(Date.now() / 1000);

  const ids = friendIds
    .map((fid) => (typeof fid === 'object' ? fid.account_id || fid.uid : fid))
    .filter(Boolean);
  const friends = (await getRepo().getByIds(ids)).map((acc) => accountToPresence(acc, now));
  ctx.logger.info(`[ported] GetFriend uid=${account.uid} -> ${friends.length} friends`);
  return { friends, star_friends: [], friends_alias_info: [] };
}

module.exports = {
  endpoint: 'GetFriend',
  reqType: null,
  resType: null,
  handler
};
