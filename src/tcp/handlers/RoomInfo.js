'use strict';

// ROOM / ROOMINFO (RoomInfoReq -> RoomInfo). The client's ONLY caller of RequestRoomInfo is the
// lobby's OnUIInit on re-entry, so a ROOM/12 poll is always a post-match RETURN: rooms.reenter
// answers with the current RoomInfo and, if the room was left INGAME by START, flips it back to
// WAITING (host can START again, room re-lists). A missing room -> ret NOROOM -> the client's
// PopupErrWindow shows the error (that used to be "This match doesn't exist" when START deleted it).
const { EProtocol, ECustomRoom, ECustomRoomErr } = require('../protocol');
const rooms = require('../rooms');

async function handler(reqObj, ctx) {
  const room = await rooms.reenter(reqObj.room_id);
  if (!room) { ctx.ret = ECustomRoomErr.NOROOM; return {}; }
  return rooms.toRoomInfo(room);
}

module.exports = {
  protocol: EProtocol.ROOM,       // 14
  subcmd: ECustomRoom.ROOMINFO,   // 12
  reqType: 'tcp.RoomInfoReq',
  resType: 'tcp.RoomInfo',
  handler
};
