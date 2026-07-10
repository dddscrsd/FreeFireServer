'use strict';

// ROOM / START (RoomStartReq{room_id}). The owner launches the match. START(11) is a client
// no-op reply, so we don't answer it — we push MATCHMAKINGSUSS_NTF(ROOM/16) = MatchmakingSussNtf
// to EACH member. The client's case-16 handler is byte-identical to the matchmaker path: it sets
// GameFacade.GameServer* from the suss and connects to server_addr with that member's
// prepare_token (LoadMPWaitingGame). We reuse the matchmaker's token/fleet/match-id helpers, so a
// room match is just a matchmaker match whose players come from the room instead of the queue.
// match_mode 6 = the CS ranking/waiting HUD (same value the matchmaker forces).
const { EProtocol, ECustomRoom } = require('../protocol');
const rooms = require('../rooms');
const matchmaker = require('../matchmaker');
const fleet = require('../fleet');
const { getRepo } = require('../../db/repo');

const MATCH_SECRET = '00112233445566778899aabbccddeeff'; // 16-byte TEA key; must match the match server (main.go secretHex)
const ROOM_MATCH_MODE = 6; // GameFacade.GameServerMatchMode 6 = CS ranking/waiting HUD

async function handler(reqObj, ctx) {
  return rooms.runOp(ctx, async () => {
    const { room } = await rooms.startMatch(ctx.account.uid, reqObj.room_id);
    const matchId = await matchmaker.nextMatchId();
    const serverAddr = fleet.pickServer() || matchmaker.staticAddr();

    for (const m of room.members) {
      if (!m.account_id) continue;
      // the host is ctx.account (fresh); look the others up for their cosmetics.
      const account = m.account_id === Number(ctx.account.uid)
        ? ctx.account
        : await getRepo().getById(m.account_id).catch(() => null);
      if (!account) { ctx.logger.warn(`[room] start uid=${m.account_id} not found — skipping their handoff`); continue; }
      const prepareToken = matchmaker.buildPrepareToken(account, matchId);
      rooms.pushToAccount(m.account_id, ECustomRoom.MATCHMAKINGSUSS_NTF, 'MatchmakingSussNtf', {
        match_id: matchId,
        server_addr: serverAddr,
        secret: MATCH_SECRET,
        prepare_token: prepareToken,
        sleep_ms: 1,
        map_id: room.map_id,
        game_mode: room.game_mode,
        group_mode: room.group_mode,
        match_mode: ROOM_MATCH_MODE,
        difficulty: 0,
        use_cache: false,
        first_login: false
      });
    }
    ctx.logger.info(`[room] START match=${matchId} room=${room.id} -> ${serverAddr} players=${room.members.length}`);
    return null; // START(11) reply is a client no-op; the suss(16) pushes drive the launch
  });
}

module.exports = {
  protocol: EProtocol.ROOM,        // 14
  subcmd: ECustomRoom.START,       // 11
  reqType: 'tcp.RoomStartReq',
  handler
};
