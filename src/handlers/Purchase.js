/**
 * Purchase  (CSPurchaseReq -> CSPurchaseRes)
 *
 * Ported from ported_3.js (handlePurchase). reference: purchase @ htpp.py:3968 —
 * grants the purchased item(s) for free and echoes back the (effectively
 * unlimited) wallet. We mirror that: append the item to the backpack and report
 * a no-cost transaction.
 */

'use strict';

const { requireAccount, nowSecs } = require('./_shared');

function handlePurchase(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};

  const itemId = Number(reqObj.store_item_id || 0);
  const cnt = Number(reqObj.cnt || 1);

  if (itemId) {
    if (!account.backpack) account.backpack = { items: [] };
    if (!Array.isArray(account.backpack.items)) account.backpack.items = [];
    account.backpack.items.push({ id: itemId, cnt });
    ctx.savePlayer();
  }

  ctx.logger.info(`[ported_3] Purchase uid=${account.uid} item=${itemId} cnt=${cnt}`);

  return {
    data: {
      trans_id: nowSecs(),
      add_item_list: itemId ? [{ id: itemId, cnt }] : [],
      del_item_list: [],
      coins_delta: 0,
      gems_delta: 0
    },
    coins: account.coins || 0,
    gems: account.gems || 0
  };
}

module.exports = {
  endpoint: 'Purchase',
  reqType: 'CSPurchaseReq',
  resType: 'CSPurchaseRes',
  handler: handlePurchase
};
