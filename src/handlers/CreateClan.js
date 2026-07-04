/**
 * CreateClan  (CSCreateClanReq -> CSClanIDRes)
 *
 * Ported verbatim from ported_0.js (handleCreateClan).
 * reference: create_clan @ htpp.py:3856 — create a clan owned by the caller.
 * The reference returned empty bytes; we mint a clan id, attach it to the
 * player and return it via the resolved CSClanIDRes type.
 */

'use strict';

const { requireAccount, nowSecs } = require('./_shared');

function handler(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};

  const clanId = nowSecs(); // simple monotonic-ish id
  account.clan = {
    id: clanId,
    name: reqObj.clan_name || 'Clan',
    role: 'captain',
    rank: 1,
    points: 0,
    members: [account.uid]
  };
  ctx.savePlayer();
  ctx.logger.info(`[ported_0] CreateClan uid=${account.uid} clan_id=${clanId} name="${account.clan.name}"`);

  return { clan_id: clanId, clan_channel_secret: '' };
}

module.exports = {
  endpoint: 'CreateClan',
  reqType: 'CSCreateClanReq',
  resType: 'CSClanIDRes',
  handler
};
