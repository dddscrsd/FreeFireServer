/**
 * GetFriendRequestList  (-> AccountInfoBasicBundleRes)
 * reference: get_friend_request_list @ htpp.py:6947 — return AccountInfoBasic
 * for every uid in the caller's pending `requests` list. Ported from ported_0.js.
 */

'use strict';

const { getRepo } = require('../db/repo');
const { requireAccount, buildAccountInfoBasic } = require('./_shared');

async function handleGetFriendRequestList(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};

  const reqUids = await getRepo().getRequestIds(account.uid);
  const infos = (await getRepo().getByIds(reqUids)).map((other) => buildAccountInfoBasic(other));
  ctx.logger.info(`[ported_0] GetFriendRequestList uid=${account.uid} count=${infos.length}`);
  return { infos };
}

module.exports = {
  endpoint: 'GetFriendRequestList',
  reqType: null,
  resType: 'AccountInfoBasicBundleRes',
  handler: handleGetFriendRequestList
};
