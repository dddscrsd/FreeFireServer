/**
 * GetClanMembers  (-> ClanMemberListWithAccountInfo)
 *
 * Ported from ported_8.js (handleGetClanMembers).
 * reference: empty stub @ py 6029. Build the member list from the caller's clan
 * doc if present, otherwise return an empty list.
 */

'use strict';

const player = require('../db/player');
const { requireAccount, accountToBasic } = require('./_shared');

function handleGetClanMembers(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  const clan = account.clan || {};
  const clanId = reqObj.clan_id || clan.id || 0;
  const members = Array.isArray(clan.members) ? clan.members : [];

  const member_list = [];
  for (const m of members) {
    const memberId = typeof m === 'object' ? m.account_id || m.uid : m;
    const acc = memberId ? player.getById(memberId) : null;
    member_list.push({
      basic_info: acc ? accountToBasic(acc) : { account_id: memberId || 0 },
      clan_id: clanId,
      member_type: (typeof m === 'object' && m.member_type) || 0,
      region: acc ? acc.region || '' : ''
    });
  }
  return { member_list };
}

module.exports = {
  endpoint: 'GetClanMembers',
  reqType: null,
  resType: null,
  handler: handleGetClanMembers
};
