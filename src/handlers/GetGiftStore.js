/**
 * GetGiftStore  (-> CSGetGiftStoreRes)
 *
 * Ported from ported_8.js (handleGetGiftStore).
 * reference: get_gift_store @ htpp.py:3952 — returns a JSON gift_items list, but
 * our 1.70 CSGetGiftStoreRes carries store metadata instead. Populate sensible
 * store fields.
 */

'use strict';

const { requireAccount } = require('./_shared');

function handler(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  return {
    store_id: 1,
    open_time: 0,
    close_time: 0,
    is_time_show: false,
    giver_level: 1,
    receiver_level: 1,
    gift_time_limited: 0,
    gift_num_limited: 0
  };
}

module.exports = {
  endpoint: 'GetGiftStore',
  reqType: null,
  resType: 'CSGetGiftStoreRes',
  handler
};
