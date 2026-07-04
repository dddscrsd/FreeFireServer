/**
 * ChooseBanner (CSChooseBannerReq -> CSGetSelectedItemsRes)
 * reference: choose_banner @ htpp.py:2870 — persists selected_items.banner_id.
 * We additionally return the updated SelectedItems (the 1.70 resType), matching
 * the convention of the other Choose* endpoints. Ported from ported_9.js.
 */

'use strict';

const { requireAccount, buildSelectedItems } = require('./_shared');

function handleChooseBanner(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  if (reqObj && reqObj.banner_id != null) {
    account.selected_items = account.selected_items || {};
    account.selected_items.banner_id = reqObj.banner_id;
    ctx.savePlayer();
  }
  return { items: buildSelectedItems(account) };
}

module.exports = {
  endpoint: 'ChooseBanner',
  reqType: null,
  resType: null,
  handler: handleChooseBanner
};
