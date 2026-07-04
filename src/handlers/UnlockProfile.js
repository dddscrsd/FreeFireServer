/**
 * UnlockProfile (CSUnlockProfileReq -> CSUnlockProfileRes)
 *
 * Ported verbatim from ported_9.js (handleUnlockProfile).
 * reference: unlock_profile / build_unlock_profile_protobuf @ htpp.py:4399/4345
 * — return the currently selected profile in affected_profiles.
 */

'use strict';

const { requireAccount, selectedProfile, buildAvatarProfile } = require('./_shared');

function handler(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  const p = selectedProfile(account);
  return {
    affected_profiles: p ? [buildAvatarProfile(p)] : [],
    award_items: []
  };
}

module.exports = {
  endpoint: 'UnlockProfile',
  reqType: null,
  resType: null,
  handler
};
