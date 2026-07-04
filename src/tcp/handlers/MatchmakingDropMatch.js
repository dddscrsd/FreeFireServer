'use strict';

/**
 * MATCHMAKING / DROPMATCH  (fire-and-forget, no reply)
 *
 * The client sends DROPMATCH when it abandons a formed match (e.g. it couldn't
 * reach the game server). It has no client-side response handler for this cmd
 * and does not retry, so we simply acknowledge it (and drop any lingering queue
 * entry) to keep it out of the "unrouted" log. Real match teardown belongs to
 * the match layer.
 */

const { EProtocol, EMatchmaking } = require('../protocol');
const matchmaker = require('../matchmaker');

function handler(reqObj, ctx) {
  matchmaker.cancel(ctx.account.uid);
  ctx.logger.info(`[tcp] MatchmakingDropMatch uid=${ctx.account.uid} (no reply)`);
  return null; // fire-and-forget: no MessageNotify response
}

module.exports = {
  protocol: EProtocol.MATCHMAKING,   // 3
  subcmd: EMatchmaking.DROPMATCH,    // 6
  reqType: null,
  resType: null,
  handler
};
