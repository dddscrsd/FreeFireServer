/**
 * GetPlayerPersonalShow  (AccountIDReq -> AccountPersonalShowInfo)
 * reference: get_player_personal_show @ htpp.py:6402 /
 * build_player_personal_show_protobuf @ htpp.py:6267 — return the full public
 * profile of the target (or the caller, when account_id == 0).
 *
 * Ported verbatim from ported_0.js (handleGetPlayerPersonalShow).
 */

'use strict';

const { getRepo } = require('../db/repo');
const { requireAccount, buildAccountInfoBasic, selectedProfile } = require('./_shared');

async function handleGetPlayerPersonalShow(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};

  const targetId = reqObj.account_id || 0;
  const acc = targetId ? (await getRepo().getById(targetId)) || account : account;

  const res = {
    basic_info: buildAccountInfoBasic(acc),
    ranking_leaderboard_pos: -1,
    news: [],
    history_ep_info: []
  };

  for (let eventId = 20; eventId >= 1; eventId--) {
    res.history_ep_info.push({
        ep_event_id: eventId,
        owned_pass: true,
        ep_badge: 1001000000 + eventId,
        badge_cnt: 999
      });
  }

  const prof = selectedProfile(acc);
  if (prof) {
    res.profile_info = {
      avatar_id: prof.avatar_id || 0,
      unlocked_level: prof.unlocked_level || 1,
      skin_color: prof.skin_color || 5,
      is_selected: true,
      clothes: Array.isArray(prof.clothes) ? prof.clothes : [],
      equiped_skills: Array.isArray(prof.equiped_skills) ? prof.equiped_skills : []
    };
  }

  const clan = acc.clan || {};
  if (clan.id) {
    res.clan_basic_info = {
      clan_id: clan.id,
      clan_name: clan.name || '',
      captain_id: acc.uid || 0,
      clan_level: 1,
      capacity: 50,
      member_num: Array.isArray(clan.members) ? clan.members.length : 1,
      honor_point: clan.points || 0
    };
  }

  return res;
}

module.exports = {
  endpoint: 'GetPlayerPersonalShow',
  reqType: 'AccountIDReq',
  resType: 'AccountPersonalShowInfo',
  handler: handleGetPlayerPersonalShow
};
