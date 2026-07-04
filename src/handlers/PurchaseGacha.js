/**
 * PurchaseGacha (CSLotteryReq -> CSLotteryRes)
 *
 * reference handle_PurchaseGacha @ htpp.py:6140 — draw random items (x1 or x10)
 * and return them as lottery_goods. We drop the Lucky/items.json tables and use
 * a simple uniform draw over a small pool.
 */

'use strict';

const { requireAccount, randInt } = require('./_shared');

// AwardType.ITEM in the reference gacha responses (origin/dest award type = 1).
const AWARD_TYPE_ITEM = 1;

// Small item pool for the gacha draw (faithful to the reference's "random item"
// behaviour without the 1.43 items.json / Lucky tables).
const GACHA_POOL = [
  211000000, 203000001, 204000001, 205000001, 203000166,
  203000092, 902000062, 901000036, 907000010, 102000005
];
const GACHA_LEGENDARY = 102000005;

function handlePurchaseGacha(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};

  // gacha_type === 2 indicates a x10 pull (reference checks several aliases).
  const itemCount = Number(reqObj.gacha_type) === 2 ? 10 : 1;

  const wonItems = [];
  for (let i = 0; i < itemCount; i += 1) {
    wonItems.push(GACHA_POOL[randInt(GACHA_POOL.length)]);
  }
  const hasBigReward = wonItems.includes(GACHA_LEGENDARY);

  const lotteryGoods = wonItems.map((itemId) => ({
    origin_award_type: AWARD_TYPE_ITEM,
    origin_award_id: itemId,
    origin_award_num: 1,
    dest_award_type: AWARD_TYPE_ITEM,
    dest_award_id: itemId,
    dest_award_num: 1
  }));

  return {
    lottery_goods: lotteryGoods,
    lottery_count_weekly: 1,
    next_free_time: 0,
    limit_purchase_count_one: 999,
    limit_purchase_count_ten: 999,
    not_got_num: 0,
    first_draw_reward_num: 0,
    has_big_reward: hasBigReward
  };
}

module.exports = {
  endpoint: 'PurchaseGacha',
  reqType: 'CSLotteryReq',
  resType: 'CSLotteryRes',
  handler: handlePurchaseGacha
};
