/**
 * GetPlayerRankingInfo  (CSPlayerRankingInfoReq -> CSPlayerRankingInfoRes)
 *
 * Ported from ported_1.js (handleGetPlayerRankingInfo).
 * reference: @4250 / build_player_ranking_info_protobuf @4196.
 */

'use strict';

const { requireAccount } = require('./_shared');

function handleGetPlayerRankingInfo(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  const rank = account.rank || 1;
  const points = account.ranking_points || 0;
  ctx.logger.info(`[ported_1] GetPlayerRankingInfo uid=${account.uid} rank=${rank} points=${points}`);
  return {
    account_id: account.uid,
    season_id: 7,
    rank,
    max_rank: rank,
    ranking_points: points,
    show_rank: true,
    match_token_num: account.match_token_num || 0,
    ranking_bot_points: 0,
    peak_rank_pos: 0
    // NOTE: last_season_info intentionally OMITTED. Advertising a prior season
    // (season 6 here) makes the 1.70 client pop the "past season ranking" dialog,
    // whose tier/particle animation then spawns particles unbounded and crashes
    // the game. Fresh accounts have no completed prior season, so we report none.
  };
}

module.exports = {
  endpoint: 'GetPlayerRankingInfo',
  reqType: 'CSPlayerRankingInfoReq',
  resType: 'CSPlayerRankingInfoRes',
  handler: handleGetPlayerRankingInfo
};
