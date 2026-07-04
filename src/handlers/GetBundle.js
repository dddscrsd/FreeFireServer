/**
 * GetBundle  (-> CSGetBundleRes). Reference: get_bundle @3960 returns a single
 * promo bundle (id 3001). Ported from ported_2.js.
 */

'use strict';

const { requireAccount } = require('./_shared');

function handleGetBundle(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  return {
    bundle_show: [{ id: 3001, bundles: [{ award_id: 0, award_num: 1 }] }]
  };
}

module.exports = {
  endpoint: 'GetBundle',
  reqType: null,
  resType: 'CSGetBundleRes',
  handler: handleGetBundle
};
