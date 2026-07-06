'use strict';

/**
 * MATCHMAKING / START  (MatchmakingStartReq -> MatchmakingStartNtf)
 *
 * The client sends START to begin matchmaking and RETRIES until it gets a reply.
 * We enqueue the player and immediately answer with a MatchmakingStartNtf (as
 * the START_NTF sub-command, cmd 11 — the client routes the reply by cmd, which
 * differs from the request sub-command). The actual match is delivered later by
 * the matchmaker via MatchmakingSussNtf.
 */

const { EProtocol, EMatchmaking } = require('../protocol');
const matchmaker = require('../matchmaker');

function handler(reqObj, ctx) {
  const mode = {
    map_id: (Array.isArray(reqObj.map_ids) && reqObj.map_ids[0]) || 1,
    game_mode: reqObj.game_mode || 15,
    match_mode: reqObj.match_mode || 1,
    difficulty: reqObj.difficulty || 0
  };
  matchmaker.enqueue(ctx.account, ctx.gateway, mode);
  ctx.logger.info(
    `[tcp] MatchmakingStart uid=${ctx.account.uid} game_mode=${mode.game_mode} map=${mode.map_id} ` +
    `match_mode=${mode.match_mode} -> START_NTF`
  );
  return {
    map_ids: (Array.isArray(reqObj.map_ids) && reqObj.map_ids.length) ? reqObj.map_ids : [mode.map_id],
    game_mode: mode.game_mode,
    difficulty: mode.difficulty,
    match_mode: mode.match_mode,
    avg_wait_time_sec: 15
  };
}

module.exports = {
  protocol: EProtocol.MATCHMAKING,     // 3
  subcmd: EMatchmaking.START,          // 1
  reqType: 'MatchmakingStartReq',
  resType: 'MatchmakingStartNtf',
  resCmd: EMatchmaking.START_NTF,      // reply routed as cmd 11
  handler
};
