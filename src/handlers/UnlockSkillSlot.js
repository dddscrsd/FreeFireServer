/**
 * UnlockSkillSlot  (CSUnlockSkillSlotReq -> CSUnlockSkillSlotRes)
 *
 * Ported from ported_0.js (handleUnlockSkillSlot). reference:
 * handle_UnlockSkillSlot @ htpp.py:6211 — empty stub. We echo the caller's
 * profiles back so the client can refresh its avatar roster.
 */

'use strict';

const { requireAccount } = require('./_shared');

function handler(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  const profiles = (account.profile && account.profile.profiles) || [];
  return { profiles };
}

module.exports = {
  endpoint: 'UnlockSkillSlot',
  reqType: 'CSUnlockSkillSlotReq',
  resType: 'CSUnlockSkillSlotRes',
  handler
};
