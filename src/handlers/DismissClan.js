/**
 * DismissClan  (CSClanIDReq -> Empty)
 *
 * Ported from ported_3.js (handleDismissClan).
 * reference: dismiss_clan @ htpp.py:3911 — deletes the clan if it exists.
 * Port: if the player owns/belongs to the clan named in the request, clear it
 * from the player document. Returns empty (Empty).
 */

'use strict';

const { requireAccount } = require('./_shared');

function handler(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  const clanId = Number(reqObj.clan_id || 0);
  if (account.clan && account.clan.id && account.clan.id === clanId) {
    account.clan = { id: 0, name: null, role: 'none', rank: 0, points: 0, members: [] };
    ctx.savePlayer();
    ctx.logger.info(`[ported_3] DismissClan uid=${account.uid} clan_id=${clanId}`);
  }
  return {};
}

module.exports = {
  endpoint: 'DismissClan',
  reqType: 'CSClanIDReq',
  resType: 'Empty',
  handler
};
