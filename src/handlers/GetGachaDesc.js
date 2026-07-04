/**
 * GetGachaDesc  (-> CSGetGachaDescRes)
 *
 * Ported verbatim from ported_1.js (handleGetGachaDesc).
 * reference: @4112. One gacha chest with its item list (mapped to 1.70
 * GachaShowItemsWithJackpot + ClientChestType). The legendary item gets
 * reward_level 3 (gold glow).
 */

'use strict';

const GACHA_LEGENDARY = 203000181;
const GACHA_ITEM_IDS = [
  GACHA_LEGENDARY,
  101000001, 101000002, 101000003, 101000004, 101000005,
  101000006, 101000007, 101000008, 101000009, 101000010,
  101000011, 101000012, 101000013, 101000014, 101000015,
  102000001, 102000002, 102000003, 102000004, 102000005
];

function handleGetGachaDesc(reqObj, ctx) {
  ctx.logger.info('[ported_1] GetGachaDesc');
  const items = GACHA_ITEM_IDS.map((itemId) => ({
    item_id: itemId,
    is_show: true,
    item_num: 1,
    reward_level: itemId === GACHA_LEGENDARY ? 3 : 0
  }));
  return {
    gacha_desc_list: [
      {
        chest_id: 1001,
        chest_type: {
          chest_id: 1001,
          priority: 1,
          coin_type: 2, // GEMS
          once_price: 60,
          ten_price: 540
        },
        item_list_with_jackpot: [{ items, jackpot: 0 }]
      }
    ]
  };
}

module.exports = {
  endpoint: 'GetGachaDesc',
  reqType: null,
  resType: 'CSGetGachaDescRes',
  handler: handleGetGachaDesc
};
