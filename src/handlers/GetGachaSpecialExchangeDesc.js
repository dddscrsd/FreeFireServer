/**
 * GetGachaSpecialExchangeDesc
 *   (CSGetGachaSpecialExchangeDescReq -> CSGetGachaSpecialExchangeDescRes)
 *
 * Ported from ported_4.js (handleGetGachaSpecialExchangeDesc).
 * reference @ htpp.py:6079 — return one ChestSpecialExchangeDesc.
 */

'use strict';

function handler(reqObj) {
  const language = (reqObj && reqObj.language) || 'pt-br';
  return {
    chest_special_exchange_descs: [
      {
        forge_tab_id: 1,
        tab_name: 'Gacha Especial',
        item_id: 101,
        item_num: 1,
        sort_id: 1,
        added_time: '2026-03-27',
        expire_time: '2026-04-27',
        limited_purchase_times: 10,
        language,
        image_url: '',
        is_show: true,
        reward_level: 5,
        real_image_url: '',
        purchase_times: 0
      }
    ]
  };
}

module.exports = {
  endpoint: 'GetGachaSpecialExchangeDesc',
  reqType: null,
  resType: null,
  handler
};
