/**
 * GetAvatarSkillSlots  (— -> Empty)
 *
 * Ported from ported_9.js (handleGetAvatarSkillSlots). reference:
 * handle_GetAvatarSkillSlots @ htpp.py:6216 — empty stub. No dedicated 1.70
 * resType could be resolved, so we respond with Empty.
 */

'use strict';

function handleGetAvatarSkillSlots() {
  return {};
}

module.exports = {
  endpoint: 'GetAvatarSkillSlots',
  reqType: null,
  resType: 'Empty',
  handler: handleGetAvatarSkillSlots
};
