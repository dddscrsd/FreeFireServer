/**
 * AccountMatchStats  (CSGetAccountMatchStatsReq -> CSGetAccountMatchStatsRes)
 *
 * Ported from login.js (handleAccountMatchStats). Returns a default-constructed
 * match-stats response (empty income / stats).
 */

'use strict';

// CSGetAccountMatchStatsReq -> CSGetAccountMatchStatsRes (default-constructed).
function handler() {
  return { income: {}, stats: {} };
}

module.exports = {
  endpoint: 'AccountMatchStats',
  reqType: 'CSGetAccountMatchStatsReq',
  resType: 'CSGetAccountMatchStatsRes',
  handler
};
