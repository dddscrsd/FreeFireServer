/**
 * GetAccountInfoByAccountID  (AccountIDReq -> AccountInfoBasic)
 *
 * Ported from ported_0.js (handleGetAccountInfoByAccountID). reference:
 * get_account_info_by_account_id @ htpp.py:3706 — look up an account by its uid
 * and return its AccountInfoBasic.
 */

'use strict';

const { getRepo } = require('../db/repo');
const { requireAccount, buildAccountInfoBasic, DEFAULT_REGION } = require('./_shared');

async function handleGetAccountInfoByAccountID(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  const targetId = reqObj.account_id || 0;
  const target = targetId ? await getRepo().getById(targetId) : null;
  if (!target) {
    ctx.logger.warn(`[ported_0] GetAccountInfoByAccountID: uid=${targetId} not found`);
    return {};
  }
  const info = buildAccountInfoBasic(target);
  // Present the searched player in the REQUESTER's region so the client's add-friend
  // "same region" gate passes. This is a single-server deployment (one shared world),
  // but region is per-account (ChooseRegion / IP), so a cross-region search would
  // otherwise be blocked from friending. `account.region || DEFAULT_REGION` mirrors
  // exactly what GetLoginData handed the client as its own local region.
  info.region = account.region || DEFAULT_REGION;
  return info;
}

module.exports = {
  endpoint: 'GetAccountInfoByAccountID',
  reqType: 'AccountIDReq',
  resType: 'AccountInfoBasic',
  handler: handleGetAccountInfoByAccountID
};
