/**
 * SelectProfile  (CSSelectProfileReq -> CSSelectProfileRes)
 *
 * Reference: select_profile @ htpp.py:2810. Flag the requested avatar as
 * selected (clearing the others), sync selected_items from it, persist, and
 * return the selected AvatarProfile. (sync_selected_items_from_profile
 * @ htpp.py:842)
 *
 * Ported from ported_6.js (handleSelectProfile), preserving logic exactly.
 */

'use strict';

const { requireAccount, syncSelectedItemsFromProfile } = require('./_shared');

function handleSelectProfile(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};

  const selectedId = reqObj.avatar_id;
  const profiles =
    (account.profile && account.profile.profiles) || [];

  let selected = null;
  for (const profile of profiles) {
    const isSel = profile.avatar_id === selectedId;
    profile.is_selected = isSel;
    if (isSel) selected = profile;
  }

  if (!selected) {
    ctx.logger.warn(`[ported_6] SelectProfile avatar not found: ${selectedId}`);
    return {};
  }

  syncSelectedItemsFromProfile(account, selected);
  ctx.savePlayer();
  ctx.logger.info(`[ported_6] SelectProfile uid=${account.uid} avatar_id=${selectedId}`);

  return {
    profile: {
      avatar_id: selected.avatar_id || 0,
      unlocked_level: selected.unlocked_level || 1,
      skin_color: selected.skin_color || 0,
      clothes: selected.clothes || [],
      equiped_skills: (selected.equiped_skills || []).map((s) => ({
        slot_id: s.slot_id || 0,
        skill_id: s.skill_id || 0
      })),
      is_selected: true
    }
  };
}

module.exports = {
  endpoint: 'SelectProfile',
  reqType: 'CSSelectProfileReq',
  resType: 'CSSelectProfileRes',
  handler: handleSelectProfile
};
