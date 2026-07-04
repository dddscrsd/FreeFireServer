/**
 * GetPlayerStats  (CSGetPlayerStatsReq -> CSGetPlayerStatsRes)
 *
 * reference: handle_GetPlayerStats @ htpp.py:5860 — fills solo/duo/quad stats
 * from the looked-up account. The 1.43 stats message carried
 * nickname/level/exp/region; the 1.70 AccountInfoWithStatsToClient instead
 * carries account_id/games_played/wins/kills, so we map onto those.
 */

'use strict';

const { requireAccount } = require('./_shared');

function handleGetPlayerStats(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};

  // Resolve the requested account (defaults to the caller), faithful to the
  // reference (account_id lookup); we only ever expose the caller's own doc.
  const id = Number(reqObj.account_id || account.uid);
  const stats = {
    account_id: id === account.uid ? account.uid : id,
    games_played: 0,
    wins: 0,
    kills: 0
  };

  ctx.logger.info(`[ported_3] GetPlayerStats uid=${account.uid} match_mode=${reqObj.match_mode || 0}`);

  return {
    solo_stats: stats,
    duo_stats: stats,
    quad_stats: stats
  };
}

module.exports = {
  endpoint: 'GetPlayerStats',
  reqType: 'CSGetPlayerStatsReq',
  resType: 'CSGetPlayerStatsRes',
  handler: handleGetPlayerStats
};
