/**
 * GetProfiles  (AvatarProfile -> CSGetProfileListRes)
 * reference: get_profiles @ htpp.py:2385 -> filters blocked avatars, ensures a
 * selected profile exists, persists the fix. (Reference returns empty bytes; we
 * also return the filtered roster, which is what resType CSGetProfileListRes
 * carries.) Ported from ported_5.js.
 */

'use strict';

const { requireAccount } = require('./_shared');

// Avatars the reference hides from the 1.17+ client (get_profiles).
const BLOCKED_AVATARS = [102000008];

function handleGetProfiles(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};

  const all = (account.profile && account.profile.profiles) || [];
  const filtered = all.filter((p) => {
    const avatarId = p.avatar_id || 0;
    const skillId =
      p.equiped_skills && p.equiped_skills[0] ? p.equiped_skills[0].skill_id || 0 : 0;
    return !BLOCKED_AVATARS.includes(avatarId) && !BLOCKED_AVATARS.includes(skillId);
  });

  // Ensure exactly one profile is flagged selected (reference [FIX]).
  if (filtered.length && !filtered.some((p) => p.is_selected)) {
    filtered[0].is_selected = true;
    ctx.savePlayer();
    ctx.logger.info(
      `[ported_5] GetProfiles uid=${account.uid} forced selected avatar=${filtered[0].avatar_id}`
    );
  }

  return { profiles: filtered };
}

module.exports = {
  endpoint: 'GetProfiles',
  reqType: null,
  resType: null,
  handler: handleGetProfiles
};
