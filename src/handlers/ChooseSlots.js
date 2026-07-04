/**
 * ChooseSlots  (CSChooseSlotsReq -> CSGetSelectedItemsRes)
 * reference: choose_slots @ htpp.py:2961 -> updates selected_items.slots and
 * returns the wrapped SelectedItems.
 *
 * Ported from ported_5.js (handleChooseSlots), preserving logic exactly. The
 * original registered ChooseSlots with no type overrides, so reqType/resType
 * are null.
 */

'use strict';

const { requireAccount } = require('./_shared');

function handleChooseSlots(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};

  const slots = Array.isArray(reqObj.slots) ? reqObj.slots : [];
  const sel = account.selected_items || (account.selected_items = {});
  sel.slots = slots;
  ctx.savePlayer();

  ctx.logger.info(`[ported_5] ChooseSlots uid=${account.uid} slots=${slots.length}`);

  // Mirror reference: return only the scalar/array fields it set on `items`
  // (skip loadouts/emotes which are nested message types).
  return {
    items: {
      avatar_id: sel.avatar_id || 0,
      skin_color: sel.skin_color || 0,
      banner_id: sel.banner_id || 0,
      head_pic: sel.head_pic || 0,
      clothes: Array.isArray(sel.clothes) ? sel.clothes : [],
      slots,
      shows: Array.isArray(sel.shows) ? sel.shows : []
    }
  };
}

module.exports = {
  endpoint: 'ChooseSlots',
  reqType: null,
  resType: null,
  handler: handleChooseSlots
};
