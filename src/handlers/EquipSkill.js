/**
 * EquipSkill  (CSEquipSkillReq -> CSEquipSkillRes)  [ref @4441]
 *
 * Ported from ported_1.js (handleEquipSkill). Reference reads
 * avatar_id/skill_id/slot_id (no-op stub). We apply the equip to the matching
 * profile and return the updated profile list.
 */

'use strict';

const { requireAccount } = require('./_shared');

function handleEquipSkill(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  const avatarId = reqObj && reqObj.avatar_id;
  const skillId = reqObj && reqObj.skill_id;
  const slotId = (reqObj && reqObj.slot_id) || 0;
  const profiles = (account.profile && account.profile.profiles) || [];

  if (avatarId != null && skillId != null) {
    const prof = profiles.find((p) => String(p.avatar_id) === String(avatarId));
    if (prof) {
      prof.equiped_skills = prof.equiped_skills || [];
      const slot = prof.equiped_skills.find((s) => String(s.slot_id) === String(slotId));
      if (slot) slot.skill_id = skillId;
      else prof.equiped_skills.push({ skill_id: skillId, slot_id: slotId });
      ctx.savePlayer();
      ctx.logger.info(`[ported_1] EquipSkill uid=${account.uid} avatar=${avatarId} skill=${skillId} slot=${slotId}`);
    }
  }
  return { profiles };
}

module.exports = {
  endpoint: 'EquipSkill',
  reqType: 'CSEquipSkillReq',
  resType: 'CSEquipSkillRes',
  handler: handleEquipSkill
};
