/**
 * EPPurchase  (idx 28, py 3457)  -> CSEPPurchaseRes (empty message)
 *   reference: marks the user's elite pass as owned, returns empty. Ported from
 *   ported_8.js.
 */

'use strict';

const { requireAccount } = require('./_shared');

function handleEPPurchase(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  account.elite_pass = true;
  ctx.savePlayer();
  ctx.logger.info(`[ported] EPPurchase uid=${account.uid} -> elite_pass=true`);
  return {};
}

module.exports = {
  endpoint: 'EPPurchase',
  reqType: null,
  resType: null,
  handler: handleEPPurchase
};
