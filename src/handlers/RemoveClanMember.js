/**
 * RemoveClanMember (CSRemoveClanMemberReq -> Empty)
 *
 * reference: handle_RemoveClanMember @ htpp.py:6009 — empty stub. We faithfully
 * drop the removee from the caller's clan member list when present.
 */

'use strict';

const { requireAccount } = require('./_shared');

function handleRemoveClanMember(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  const clan = account.clan;
  const removee = Number(reqObj.removee_id || 0);
  if (clan && Array.isArray(clan.members) && removee) {
    const before = clan.members.length;
    clan.members = clan.members.filter(
      (m) => Number(m && m.account_id != null ? m.account_id : m) !== removee
    );
    if (clan.members.length !== before) ctx.savePlayer();
  }
  return {};
}

module.exports = {
  endpoint: 'RemoveClanMember',
  reqType: null,
  resType: null,
  handler: handleRemoveClanMember
};
