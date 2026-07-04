/**
 * GetFriendIDs  (CSGetFriendIDsReq -> AccountIDSlice)
 *
 * Ported from ported_9.js (handleGetFriendIDs).
 * reference: get_friend @ htpp.py:7012 — returns the account's friend list.
 * AccountIDSlice carries just the ids.
 */

'use strict';

const { requireAccount } = require('./_shared');

function handleGetFriendIDs(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return { account_ids: [] };
  const ids = (Array.isArray(account.friends) ? account.friends : []).map((f) => Number(f));
  return { account_ids: ids };
}

module.exports = {
  endpoint: 'GetFriendIDs',
  reqType: null,
  resType: null,
  handler: handleGetFriendIDs
};
