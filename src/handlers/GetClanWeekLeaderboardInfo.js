/**
 * GetClanWeekLeaderboardInfo  (-> CSGetClanWeekLeaderboardInfoRes)  [ref @5855]
 * Reference is an empty stub.
 *
 * Ported verbatim from ported_1.js (handleGetClanWeekLeaderboardInfo).
 */

'use strict';

const { nowSecs } = require('./_shared');

function handleGetClanWeekLeaderboardInfo(reqObj, ctx) {
  ctx.logger.info('[ported_1] GetClanWeekLeaderboardInfo');
  return { main_key: 0, next_refresh_time: nowSecs() + 7 * 24 * 3600 };
}

module.exports = {
  endpoint: 'GetClanWeekLeaderboardInfo',
  reqType: null,
  resType: 'CSGetClanWeekLeaderboardInfoRes',
  handler: handleGetClanWeekLeaderboardInfo
};
