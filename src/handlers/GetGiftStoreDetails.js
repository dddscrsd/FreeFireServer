/**
 * GetGiftStoreDetails  (-> CSGetGiftStoreDetailsRes)
 *
 * Ported from ported_9.js (handleGetGiftStoreDetails). reference:
 * handle_GetGiftStoreDetails @ htpp.py:6231 — store_id=1,
 * send_gift_times_today=0.
 */

'use strict';

function handler() {
  return { store_id: 1, send_gift_times_today: 0, items: [] };
}

module.exports = {
  endpoint: 'GetGiftStoreDetails',
  reqType: null,
  resType: 'CSGetGiftStoreDetailsRes',
  handler
};
