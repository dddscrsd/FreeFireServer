/**
 * EPClaimBadge  (CSEPClaimBadgeReq -> CSEPClaimBadgeRes)
 * reference: ep_claim_badge @ htpp.py:3478 -> marks challenge claimed, +10 badge.
 *
 * Ported verbatim from ported_5.js (handleEPClaimBadge). Registered without
 * explicit req/res types in the source, so both are null.
 */

'use strict';

const { requireAccount } = require('./_shared');

function handleEPClaimBadge(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  account.ep_badge_count = (account.ep_badge_count || 0) + 10;
  ctx.savePlayer();
  ctx.logger.info(`[ported_5] EPClaimBadge uid=${account.uid} badge_count=${account.ep_badge_count}`);
  return {};
}

module.exports = {
  endpoint: 'EPClaimBadge',
  reqType: null,
  resType: null,
  handler: handleEPClaimBadge
};
