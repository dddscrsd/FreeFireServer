/**
 * ModifyClanInfo  (CSModifyClanInfoReq -> Empty)
 *
 * Ported from ported_9.js (handleModifyClanInfo).
 * reference: modify_clan_info @ htpp.py:3892 — update the caller's clan info
 * (announcement/slogan in 1.70) when it matches the caller's clan.
 */

'use strict';

const { requireAccount } = require('./_shared');

function handler(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  const clan = account.clan;
  if (clan && Number(clan.id) === Number(reqObj.clan_id || 0)) {
    if (reqObj.announcement != null) clan.announcement = String(reqObj.announcement);
    if (reqObj.slogan != null) clan.slogan = String(reqObj.slogan);
    ctx.savePlayer();
  }
  return {};
}

module.exports = {
  endpoint: 'ModifyClanInfo',
  reqType: null,
  resType: null,
  handler
};
