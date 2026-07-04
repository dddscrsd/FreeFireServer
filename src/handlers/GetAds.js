/**
 * GetAds  (Empty -> CSGetAdvertRes)
 * reference: handle_GetAds @ htpp.py:6064 (empty stub). No dedicated "Ads"
 * message exists in 1.70 protos; resolved best-effort to CSGetAdvertRes (the
 * closest advert-list response). Returns an empty advert list. Ported from
 * ported_3.js.
 */

'use strict';

const { requireAccount } = require('./_shared');

function handleGetAds(reqObj, ctx) {
  if (!requireAccount(ctx)) return {};
  return {}; // advert_items: []
}

module.exports = {
  endpoint: 'GetAds',
  reqType: 'Empty',
  resType: 'CSGetAdvertRes',
  handler: handleGetAds
};
