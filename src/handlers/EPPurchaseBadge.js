/**
 * EPPurchaseBadge  (CSEPPurchaseBadgeReq -> CSEPPurchaseBadgeRes)
 *
 * Ported verbatim from ported_9.js (handleEPPurchaseBadge).
 * reference: ep_purchase_badge @ htpp.py:3491 — grants a large badge count.
 */

'use strict';

const { requireAccount } = require('./_shared');

function handleEPPurchaseBadge(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  account.ep_badge_count = (account.ep_badge_count || 0) + 9999;
  ctx.savePlayer();
  return {}; // CSEPPurchaseBadgeRes is an empty message.
}

module.exports = {
  endpoint: 'EPPurchaseBadge',
  reqType: null,
  resType: null,
  handler: handleEPPurchaseBadge
};
