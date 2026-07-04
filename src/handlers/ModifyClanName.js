/**
 * ModifyClanName  (CSModifyClanNameReq -> Empty)
 *
 * Ported verbatim from ported_0.js (handleModifyClanName).
 * reference: handle_ModifyClanName @ htpp.py:6034 — empty stub. We additionally
 * update the caller's stored clan name when they belong to that clan.
 */

'use strict';

const { requireAccount } = require('./_shared');

function handleModifyClanName(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  if (account.clan && account.clan.id && reqObj.clan_name) {
    account.clan.name = String(reqObj.clan_name);
    ctx.savePlayer();
  }
  return {};
}

module.exports = {
  endpoint: 'ModifyClanName',
  reqType: 'CSModifyClanNameReq',
  resType: 'Empty',
  handler: handleModifyClanName
};
