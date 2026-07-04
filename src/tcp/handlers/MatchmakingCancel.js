'use strict';

/**
 * MATCHMAKING / CANCEL  (-> MatchmakingStopNtf)
 *
 * The client sends CANCEL when the player leaves the queue. We remove them from
 * the matchmaker and confirm with a STOP_NTF (cmd 14; its content is unused by
 * the client). No request body is decoded.
 */

const { EProtocol, EMatchmaking } = require('../protocol');
const matchmaker = require('../matchmaker');

function handler(reqObj, ctx) {
  matchmaker.cancel(ctx.account.uid);
  ctx.logger.info(`[tcp] MatchmakingCancel uid=${ctx.account.uid} -> STOP_NTF`);
  return {}; // STOP_NTF: client only reads `ret`, content is ignored
}

module.exports = {
  protocol: EProtocol.MATCHMAKING,     // 3
  subcmd: EMatchmaking.CANCEL,         // 2
  reqType: null,
  resType: null,                       // empty content
  resCmd: EMatchmaking.STOP_NTF,       // reply routed as cmd 14
  handler
};
