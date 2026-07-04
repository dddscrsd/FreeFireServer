/**
 * QuitClan  (CSQuitClanReq -> Empty)
 *
 * Ported from ported_5.js (handleQuitClan). reference: quit_clan @ htpp.py:3907
 * (returns empty). We clear clan membership.
 *
 * (registered with no explicit types in the source, so reqType/resType are null
 * and resolved from endpoint_map.json by the router.)
 */

'use strict';

const { requireAccount } = require('./_shared');

function handler(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  account.clan = { id: 0, name: null, role: 'none', rank: 0, points: 0, members: [] };
  ctx.savePlayer();
  ctx.logger.info(`[ported_5] QuitClan uid=${account.uid}`);
  return {};
}

module.exports = {
  endpoint: 'QuitClan',
  reqType: null,
  resType: null,
  handler
};
