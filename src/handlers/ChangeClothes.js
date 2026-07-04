/**
 * ChangeClothes  (CSChangeClothesReq -> CSChangeClothesRes)
 *
 * Ported from ported_7.js (handleChangeClothes).
 * reference: change_clothes @ htpp.py:2732. Find the profile for the requested
 * avatar (or the currently-selected one when avatar_id is omitted), apply the
 * new clothes/skin_color, sync selected_items when it is the selected profile,
 * persist, and return the updated AvatarProfile wrapped in CSChangeClothesRes.
 */

'use strict';

const { requireAccount, mapProfileToAvatarProfile } = require('./_shared');

function handleChangeClothes(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};

  const profiles = (account.profile && account.profile.profiles) || [];
  let avatarId = reqObj.avatar_id;

  // avatar_id omitted -> use the currently-selected profile (reference parity).
  if (!avatarId) {
    const sel = profiles.find((p) => p.is_selected);
    if (sel) avatarId = sel.avatar_id;
  }

  const profile = profiles.find((p) => p.avatar_id === avatarId);
  if (!profile) {
    ctx.logger.warn(`[ported_7] ChangeClothes: avatar ${avatarId} not found uid=${account.uid}`);
    return {}; // reference returns 404; benign empty res here.
  }

  if (reqObj.clothes && reqObj.clothes.length) profile.clothes = reqObj.clothes.slice();
  if (reqObj.skin_color != null) profile.skin_color = reqObj.skin_color;

  // Sync selected_items from the selected profile (reference:
  // sync_selected_items_from_profile) so the lobby renders the new look.
  if (profile.is_selected) {
    account.selected_items = account.selected_items || {};
    account.selected_items.avatar_id = profile.avatar_id;
    account.selected_items.skin_color = profile.skin_color || 0;
    account.selected_items.clothes = Array.isArray(profile.clothes) ? profile.clothes.slice() : [];
  }

  ctx.savePlayer();
  ctx.logger.info(`[ported_7] ChangeClothes uid=${account.uid} avatar=${avatarId}`);

  return { profile: mapProfileToAvatarProfile(profile) };
}

module.exports = {
  endpoint: 'ChangeClothes',
  reqType: 'CSChangeClothesReq',
  resType: 'CSChangeClothesRes',
  handler: handleChangeClothes
};
