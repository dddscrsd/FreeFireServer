'use strict';

// ROOM / ROOMINFO (RoomInfoReq -> RoomInfo). A poll for the full current room state.
const { EProtocol, ECustomRoom, ECustomRoomErr } = require('../protocol');
const rooms = require('../rooms');

async function handler(reqObj, ctx) {
  const room = await rooms.get(reqObj.room_id);
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
