/**
 * GetTopNDailyLeaderboard  (Empty -> AccountLeaderboardRes)
 *
 * Ported from ported_3.js (handleGetTopNDailyLeaderboard).
 * reference: handle_GetTopNDailyLeaderboard @ htpp.py:5924 (empty stub).
 * resType resolved best-effort to AccountLeaderboardRes. Empty board.
 */

'use strict';

const { requireAccount } = require('./_shared');

function handleGetTopNDailyLeaderboard(reqObj, ctx) {
  if (!requireAccount(ctx)) return {};
  return {}; // items: [], leaderboard_size: 0
}

module.exports = {
  endpoint: 'GetTopNDailyLeaderboard',
  reqType: 'Empty',
  resType: 'AccountLeaderboardRes',
  handler: handleGetTopNDailyLeaderboard
};
