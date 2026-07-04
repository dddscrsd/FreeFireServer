/**
 * ChooseLoadout  (CSChooseLoadoutReq -> CSGetSelectedItemsRes)
 *
 * Ported verbatim from ported_2.js (handleChooseLoadout).
 * Reference is an empty stub; resType returns the player's selected items. We
 * apply the requested loadout list, persist, and echo the selected items back.
 */

'use strict';

const { requireAccount, buildSelectedItems } = require('./_shared');

function handler(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  if (Array.isArray(reqObj.loadouts) && reqObj.loadouts.length) {
    account.selected_items = account.selected_items || {};
    account.selected_items.loadouts = reqObj.loadouts.slice();
    ctx.savePlayer();
  }
  return { items: buildSelectedItems(account) };
}

module.exports = {
  endpoint: 'ChooseLoadout',
  reqType: 'CSChooseLoadoutReq',
  resType: 'CSGetSelectedItemsRes',
  handler
};
