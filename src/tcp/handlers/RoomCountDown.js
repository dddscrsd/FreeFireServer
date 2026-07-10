'use strict';

// ROOM / COUNTDOWN (RoomCountDownReq{room_id, count_down_seconds}). When the client config
// GameVarDef.CustomRoomCountDownTime != 0, the host's Start button sends THIS instead of START(11).
// We broadcast COUNTDOWN_NTF(40) = tcp.RoomCountDownNtf{count_down_seconds} to every member; their
// UICustomRoomCountDownWindow runs the timer and re-sends START(11) on finish (-> RoomStart -> suss).
// COUNTDOWN(39) is a client no-op reply, so it's driven entirely by the broadcast.
const { EProtocol, ECustomRoom, ECustomRoomErr } = require('../protocol');
const rooms = require('../rooms');

async function handler(reqObj, ctx) {
  const roomId = Number(reqObj.room_id) || (await rooms.roomIdOf(ctx.account.uid));
  const room = await rooms.get(roomId);
  if (!room) { ctx.ret = ECustomRoomErr.NOROOM; return null; }
  if (room.owner !== Number(ctx.account.uid)) { ctx.ret = ECustomRoomErr.NOTOWNER; return null; }
  const seconds = Number(reqObj.count_down_seconds || 0);
  rooms.broadcast(room, null, ECustomRoom.COUNTDOWN_NTF, 'tcp.RoomCountDownNtf', { count_down_seconds: seconds });
  ctx.logger.info(`[room] COUNTDOWN room=${room.id} seconds=${seconds} -> all members`);
  return null;
}

module.exports = {
  protocol: EProtocol.ROOM,          // 14
  subcmd: ECustomRoom.COUNTDOWN,     // 39
  reqType: 'tcp.RoomCountDownReq',
  handler
};
