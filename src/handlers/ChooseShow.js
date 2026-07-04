/**
 * ChooseShow  (CSChooseShowReq -> CSGetSelectedItemsRes)
 *
 * Ported from ported_4.js (handleChooseShow). reference handle_ChooseShow @
 * htpp.py:7471 — persist the equipped "shows" onto the account's selected_items,
 * then echo back the full SelectedItems.
 *
 * (registered with no explicit types in the source, so reqType/resType are null
 * and resolved from endpoint_map.json by the router.)
 */

'use strict';

const { requireAccount } = require('./_shared');

function handler(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};

  const shows = Array.isArray(reqObj.shows) ? reqObj.shows.map(Number) : [];

  if (!account.selected_items) account.selected_items = {};
  account.selected_items.shows = shows;
  ctx.savePlayer();

  const si = account.selected_items;
  return {
    items: {
      avatar_id: si.avatar_id || 0,
      skin_color: si.skin_color || 0,
      banner_id: si.banner_id || 0,
      head_pic: si.head_pic || 0,
      clothes: Array.isArray(si.clothes) ? si.clothes : [],
      slots: Array.isArray(si.slots) ? si.slots : [],
      shows
    }
  };
}

module.exports = {
  endpoint: 'ChooseShow',
  reqType: null,
  resType: null,
  handler
};
