/**
 * GetFriendRequestList  (-> AccountInfoBasicBundleRes)
 * reference: get_friend_request_list @ htpp.py:6947 — return AccountInfoBasic
 * for every uid in the caller's pending `requests` list. Ported from ported_0.js.
 */

'use strict';

const player = require('../db/player');
const { requireAccount, buildAccountInfoBasic } = require('./_shared');

function handleGetFriendRequestList(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};

  const reqUids = Array.isArray(account.requests) ? account.requests : [];
  const infos = [];
  for (const uid of reqUids) {
    const other = player.getById(uid);
    if (other) infos.push(buildAccountInfoBasic(other));
  }
  ctx.logger.info(`[ported_0] GetFriendRequestList uid=${account.uid} count=${infos.length}`);
  return { infos };
}

module.exports = {
  endpoint: 'GetFriendRequestList',
  reqType: null,
  resType: 'AccountInfoBasicBundleRes',
  handler: handleGetFriendRequestList
};
