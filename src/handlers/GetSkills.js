/**
 * GetSkills  (-> CSGetSkillListRes)  [ref @6044]
 *
 * Ported from ported_1.js (handleGetSkills). Reference is an empty stub. Derive
 * the account's owned skill ids from its profiles' equipped skills.
 */

'use strict';

const { requireAccount } = require('./_shared');

function handler(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  const profiles = (account.profile && account.profile.profiles) || [];
  const skills = [];
  for (const p of profiles) {
    for (const s of p.equiped_skills || []) {
      if (s.skill_id != null && !skills.includes(s.skill_id)) skills.push(s.skill_id);
    }
  }
  ctx.logger.info(`[ported_1] GetSkills uid=${account.uid} count=${skills.length}`);
  return { account_id: account.uid, skills };
}

module.exports = {
  endpoint: 'GetSkills',
  reqType: null,
  resType: 'CSGetSkillListRes',
  handler
};
