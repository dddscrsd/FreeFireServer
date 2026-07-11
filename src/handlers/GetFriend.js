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
const { getBus } = require('../bus/instance');

async function handler(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  const now = Math.floor(Date.now() / 1000);

  const ids = await getRepo().getFriendIds(account.uid);
  const accts = await getRepo().getByIds(ids);

  // Cross-layer presence: the TCP gateway (a DIFFERENT process) writes presence:<uid>
  // while a client's notification channel is connected, so here we can tell which
  // friends are actually online. Best-effort — a bus/Redis hiccup falls back to
  // "offline" and never breaks the friend list.
  let onlineSet = new Set();
  try {
    const bus = getBus();
    if (bus && ids.length) {
      onlineSet = new Set(Object.keys(await bus.getNodes(ids)).map(Number));
    }
  } catch (err) {
    ctx.logger.warn(`[ported] GetFriend presence lookup failed: ${err.message}`);
  }

  const friends = accts.map((acc) => {
    const p = accountToPresence(acc, now);
    // update_time is the "last active" presence signal: keep `now` for online
    // friends; for offline ones use their real last-login so the client shows them
    // offline / "last seen" instead of perpetually active.
    if (!onlineSet.has(acc.uid)) p.update_time = p.last_login_at || 0;
    return p;
  });
  ctx.logger.info(`[ported] GetFriend uid=${account.uid} -> ${friends.length} friends (${onlineSet.size} online)`);
  return { friends, star_friends: [], friends_alias_info: [] };
}

module.exports = {
  endpoint: 'GetFriend',
  reqType: null,
  resType: null,
  handler
};
