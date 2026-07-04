/**
 * LoginGetProfile  (Empty -> LoginProfileRes)  [public LoginGet*]
 *
 * Ported from ported_5.js (handleLoginGetProfile).
 * reference: build_login_profile_protobuf @ htpp.py:2581.
 */

'use strict';

const player = require('../db/player');

// LoginGet* endpoints are public (router does not attach ctx.account), so
// resolve the player from the Bearer token ourselves, mirroring the reference.
function accountFromToken(ctx) {
  const auth = (ctx.req && ctx.req.headers['authorization']) || '';
  const token = auth.startsWith('Bearer ') ? auth.slice(7).trim() : '';
  return token ? player.getByToken(token) : null;
}

function handleLoginGetProfile(reqObj, ctx) {
  const account = ctx.account || accountFromToken(ctx);
  if (!account) return {}; // reference returns 401/404; we respond benignly.

  const profiles = (account.profile && account.profile.profiles) || [];

  // Collect unique skill ids from equiped skills + selected loadouts (sorted).
  const skillSet = new Set();
  for (const p of profiles) {
    for (const s of p.equiped_skills || []) {
      if (s.skill_id > 0) skillSet.add(s.skill_id);
    }
  }
  const loadouts = (account.selected_items && account.selected_items.loadouts) || [];
  for (const l of loadouts) {
    if (typeof l === 'number' && l > 0) skillSet.add(l);
  }
  const skills = Array.from(skillSet).sort((a, b) => a - b);

  const pet = account.pet || {};

  return {
    profile_res: { profiles },
    skill_res: { account_id: account.uid, skills },
    pet_info: {
      id: pet.id || 0,
      name: pet.name || '',
      level: pet.level || 0,
      exp: pet.exp || 0,
      is_selected: !!pet.is_selected
    }
  };
}

module.exports = {
  endpoint: 'LoginGetProfile',
  reqType: 'Empty',
  resType: 'LoginProfileRes',
  handler: handleLoginGetProfile
};
