/**
 * GetStore  (CSGetStoreReq -> CSGetStoreRes)
 *
 * Ported from ported_2.js (handleGetStore). reference: get_store @3923 returns
 * one sample StoreDesc (store_id 1, item_id 102000006, free).
 */

'use strict';

const { requireAccount } = require('./_shared');

function handleGetStore(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  return {
    store_items: [
      {
        store_id: 1,
        sort_id: 1,
        item_id: 102000006,
        coins_price: 0,
        gems_price: 0,
        is_new: true
      }
    ]
  };
}

module.exports = {
  endpoint: 'GetStore',
  reqType: 'CSGetStoreReq',
  resType: 'CSGetStoreRes',
  handler: handleGetStore
};
