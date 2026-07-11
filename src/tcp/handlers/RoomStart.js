'use strict';

// ROOM / START (RoomStartReq{room_id}). The owner launches the match. START(11) is a client
// no-op reply, so we don't answer it — we push MATCHMAKINGSUSS_NTF(ROOM/16) = MatchmakingSussNtf
// to EACH member. The client's case-16 handler is byte-identical to the matchmaker path: it sets
// GameFacade.GameServer* from the suss and connects to server_addr with that member's
// prepare_token (LoadMPWaitingGame). We reuse the matchmaker's token/fleet/match-id helpers, so a
// room match is just a matchmaker match whose players come from the room instead of the queue.
//
// match_mode = 3 is REQUIRED (NOT 6): GameFacade::IsCustomRoom() == (GameServerMatchMode == 3), so
// 3 tags the match as a CUSTOM ROOM — it drives the custom-room result screen and the room-aware
// post-match return (the lobby re-queries ROOM/12 to re-enter the room) and keeps the match out of
// the ranked-CS stat pool. game_mode stays 15 (CS), so IsCSMode() + CS gameplay are unchanged.
const { EProtocol, ECustomRoom } = require('../protocol');
const rooms = require('../rooms');
const roomsettings = require('../roomsettings');
const matchmaker = require('../matchmaker');
const fleet = require('../fleet');
const { getBus } = require('../../bus/instance');
const { getRepo } = require('../../db/repo');

const MATCH_SECRET = '00112233445566778899aabbccddeeff'; // 16-byte TEA key; must match the match server (main.go secretHex)
const ROOM_MATCH_MODE = 3; // GameFacade.GameServerMatchMode 3 = CUSTOM ROOM (IsCustomRoom); NOT 6 (ranked CS)

async function handler(reqObj, ctx) {
  return rooms.runOp(ctx, async () => {
    const { room } = await rooms.startMatch(ctx.account.uid, reqObj.room_id);
    const matchId = await matchmaker.nextMatchId();
    const serverAddr = fleet.pickServer() || matchmaker.staticAddr();

    // Decode the host's simple-mode room_setting/room_setting2 and hand them to the match server via
    // Redis (match:<id>:settings), so it applies the round count / HP / economy / flags for THIS
    // match. Best-effort: the match keeps its const defaults when this is absent or a field is unset.
    // (Advanced-mode cs_advanced_setting is not decoded yet — a later phase.)
    try {
      const s = roomsettings.decodeRoomSettings(room);
      const payload = { flags: s.flags };
      if (s.roundCount) payload.round_count = s.roundCount;
      if (s.maxHP) payload.max_hp = s.maxHP;
      if (Array.isArray(s.economyTable)) payload.economy = s.economyTable;
      // Pass the RAW bitfields too: the match server echoes them into JoinMatchRes (cmd 100) so the
      // client applies its own client-side flags (UnlimitedAmmo/NoHud/NoAuxAim/NoSkill/FriendDmg/…).
      if (s.roomSetting) payload.room_setting = s.roomSetting;
      if (s.roomSetting2) payload.room_setting2 = s.roomSetting2;
      // Move-speed / jump-height multipliers -> PRI fields 50/52 (percent-encoded match-side).
      if (s.speedMul) payload.speed_mul = s.speedMul;
      if (s.jumpMul) payload.jump_mul = s.jumpMul;
      const bus = getBus();
      if (bus) await bus.setKey(`match:${matchId}:settings`, JSON.stringify(payload), 3600);
      ctx.logger.info(`[room] START settings match=${matchId} rounds=${s.roundCount || 'default'} hp=${s.maxHP || 'default'} speed=${s.speedMul || 'default'} jump=${s.jumpMul || 'default'} rs=0x${(s.roomSetting || 0).toString(16)}/0x${(s.roomSetting2 || 0).toString(16)} flags=${Object.entries(s.flags).filter(([, v]) => v).map(([k]) => k).join(',') || 'none'}`);
    } catch (e) {
      ctx.logger.warn(`[room] start settings write match=${matchId}: ${e.message}`);
    }

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
        // Night/dawn visual style: the client reads MatchmakingSussNtf.level_visual_style into
        // GameFacade.LevelVisualStyle (OnMsgHasLatestGameInfo @0x1b68c04); bit 0 = night, which
        // SceneGraphics reads to enable the global BRNIGHT_ON shader keyword (world darkening).
        // NOTE: this ONLY darkens the world — the SKYBOX is owned by the client's EnvWeather
        // controller (per-map night weather preset), which no server field can drive on a generic
        // CS map, so the skybox stays daytime unless the map itself ships a night preset.
        level_visual_style: room.level_visual_style || 0,
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
