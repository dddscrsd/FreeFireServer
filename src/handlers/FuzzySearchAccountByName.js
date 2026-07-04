/**
 * FuzzySearchAccountByName  (py 3594)  -> AccountInfoBasicBundleRes
 *   reference: case-insensitive partial nickname search (limit 20), each match
 *   mapped to AccountInfoBasic.
 *
 * Ported verbatim from ported_8.js (handleFuzzySearchAccountByName). Registered
 * without explicit req/res types in the source, so both are null. The prepared
 * search statement is endpoint-specific and kept here.
 */

'use strict';

const accounts = require('../db/accounts');
const player = require('../db/player');
const { requireAccount, accountToBasic } = require('./_shared');

const searchByNicknameStmt = accounts.db.prepare(
  'SELECT account_id FROM accounts WHERE nickname LIKE ? COLLATE NOCASE LIMIT 20'
);

function handleFuzzySearchAccountByName(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  const nickname = (reqObj.nickname || '').trim();
  if (!nickname) return { infos: [] };

  const rows = searchByNicknameStmt.all(`%${nickname}%`);
  const infos = [];
  for (const row of rows) {
    const acc = player.getById(row.account_id);
    if (acc) infos.push(accountToBasic(acc));
  }
  ctx.logger.info(`[ported] FuzzySearchAccountByName "${nickname}" -> ${infos.length} hits`);
  return { infos };
}

module.exports = {
  endpoint: 'FuzzySearchAccountByName',
  reqType: null,
  resType: null,
  handler: handleFuzzySearchAccountByName
};
