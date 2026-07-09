/**
 * GetAccountInfoByAccountID  (AccountIDReq -> AccountInfoBasic)
 *
 * Ported from ported_0.js (handleGetAccountInfoByAccountID). reference:
 * get_account_info_by_account_id @ htpp.py:3706 — look up an account by its uid
 * and return its AccountInfoBasic.
 */

'use strict';

const { getRepo } = require('../db/repo');
const { requireAccount, buildAccountInfoBasic } = require('./_shared');

async function handleGetAccountInfoByAccountID(reqObj, ctx) {
  if (!requireAccount(ctx)) return {};
  const targetId = reqObj.account_id || 0;
  const target = targetId ? await getRepo().getById(targetId) : null;
  if (!target) {
    ctx.logger.warn(`[ported_0] GetAccountInfoByAccountID: uid=${targetId} not found`);
    return {};
  }
  return buildAccountInfoBasic(target);
}

module.exports = {
  endpoint: 'GetAccountInfoByAccountID',
  reqType: 'AccountIDReq',
  resType: 'AccountInfoBasic',
  handler: handleGetAccountInfoByAccountID
};
