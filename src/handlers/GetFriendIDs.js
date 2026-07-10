/**
 * GetFriendIDs  (CSGetFriendIDsReq -> AccountIDSlice)
 *
 * Ported from ported_9.js (handleGetFriendIDs).
 * reference: get_friend @ htpp.py:7012 — returns the account's friend list.
 * AccountIDSlice carries just the ids.
 */

'use strict';

const { requireAccount } = require('./_shared');
const { getRepo } = require('../db/repo');

async function handleGetFriendIDs(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return { account_ids: [] };
  return { account_ids: await getRepo().getFriendIds(account.uid) };
}

module.exports = {
  endpoint: 'GetFriendIDs',
  reqType: null,
  resType: null,
  handler: handleGetFriendIDs
};
