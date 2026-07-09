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

const { getRepo } = require('../db/repo');
const { requireAccount, accountToBasic, DEFAULT_REGION } = require('./_shared');

async function handleFuzzySearchAccountByName(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  const nickname = (reqObj.nickname || '').trim();
  if (!nickname) return { infos: [] };

  const repo = getRepo();
  const ids = await repo.searchByNickname(nickname);
  const infos = [];
  for (const acc of await repo.getByIds(ids)) {
    if (!acc) continue;
    const basic = accountToBasic(acc);
    // Same-region so the client's add-friend gate passes (see GetAccountInfoByAccountID).
    basic.region = account.region || DEFAULT_REGION;
    infos.push(basic);
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
